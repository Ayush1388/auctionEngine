// The live system view shared by the Chaos lab and the Architecture map: health
// from the probes, metrics for operators, and pulses for every real event.
import { createMap, PATHS, nodeFacts } from "./archmap.js";
import { FAULTS } from "./faults.js";
import { html, $, dur, usd } from "./util.js";
import { toLot } from "./model.js";

const POLL = 2000;
const NODE_FAULT = { redis: "redis", kafka: "kafka", elasticsearch: "elasticsearch", bidding: "bidding", postgres: "postgres_slow" };
const fam = (m, name) => m?.families.find(f => f.name === name);
const sum = f => (f?.samples || []).reduce((a, s) => a + (s.value ?? 0), 0);

export function mountSystem(be, { mapRoot, panelRoot, watch = 10 }) {
  const state = { ready: null, ms: null, metrics: null, chaos: null, lastBid: null, lastSearch: null, events: 0, stopped: false, err: null };
  const subs = [];
  const listeners = new Set();
  const map = createMap(mapRoot, { onSelect: () => panel() });
  map.select("bidding");

  function health() {
    const c = state.ready?.checks || {}, f = Object.fromEntries((state.chaos?.faults || []).map(x => [x.name, x.active]));
    const br = name => {
      const s = fam(state.metrics, "auction_circuit_breaker_state")?.samples.find(x => x.labels?.name === name);
      return s ? s.value : null;
    };
    const fromCheck = k => (!(k in c) ? "off" : c[k] === "ok" ? "ok" : "bad");
    const pend = sum(fam(state.metrics, "auction_outbox_pending")), dead = sum(fam(state.metrics, "auction_outbox_dead_lettered"));
    const bidB = br("bidding");
    return {
      browser: "ok",
      gateway: state.ms == null ? "bad" : "ok",
      bidding: f.bidding || bidB === 2 ? "bad" : bidB === 1 ? "warn" : "ok",
      postgres: c.postgres && c.postgres !== "ok" ? "bad" : f.postgres_slow ? "warn" : "ok",
      outbox: dead > 0 ? "warn" : pend > 50 ? "warn" : "ok",
      kafka: fromCheck("kafka"),
      redis: fromCheck("redis"),
      elasticsearch: br("elasticsearch") === 2 ? "bad" : fromCheck("elasticsearch"),
      websocket: be.feed.state === "open" || be.feed.state === "idle" ? "ok" : "warn",
    };
  }

  function stats(id) {
    const m = state.metrics, lb = state.lastBid, ls = state.lastSearch;
    const rows = [];
    const add = (k, v) => v != null && rows.push([k, v]);
    if (id === "gateway") add("Liveness round trip", state.ms != null ? dur(state.ms) : "no answer");
    if (id === "bidding") { add("Last bid, whole call", lb?.timing?.bidding != null ? dur(lb.timing.bidding) : null); add("Last bid outcome", lb ? String(lb.status) : null); }
    if (id === "postgres" && lb?.timing?.lock != null) { const t = lb.timing; add("Last bid: lock", dur(t.lock)); add("Last bid: write and commit", dur((t.write || 0) + (t.commit || 0))); }
    if (id === "outbox") { add("Waiting", m ? String(sum(fam(m, "auction_outbox_pending"))) : null); add("Dead-lettered", m ? String(sum(fam(m, "auction_outbox_dead_lettered"))) : null); }
    if (id === "kafka") add("Consumer lag", m ? String(sum(fam(m, "auction_kafka_consumer_lag"))) : null);
    if (id === "websocket") { add("Connection", be.feed.state); add("Connections open", m ? String(sum(fam(m, "auction_ws_connections"))) : null); }
    if (id === "elasticsearch" || id === "redis") add("Search answered by", ls ? ls.backend : null);
    if (id === "elasticsearch") add("Last search", ls ? dur(ls.ms) : null);
    add("Events seen on this page", id === "websocket" ? String(state.events) : null);
    return rows;
  }

  function panel() {
    if (!panelRoot) return;
    const id = map.selected, n = id && nodeFacts[id] ? id : "bidding";
    const st = map.healthOf(n) || "ok", f = FAULTS.find(x => x.name === NODE_FAULT[n]);
    const label = { browser: "Browser", gateway: "API gateway", bidding: "Bidding service", postgres: "PostgreSQL", outbox: "Outbox worker", kafka: "Kafka", redis: "Redis", elasticsearch: "Elasticsearch", websocket: "WebSocket hub" }[n];
    const rows = stats(n);
    panelRoot.innerHTML = String(html`
      <div class="node-panel" aria-live="polite">
        <h3>${label}</h3>
        <p>${nodeFacts[n]}</p>
        <dl>
          <div><dt>Health</dt><dd>${({ ok: "Answering", warn: "Degraded", bad: "Down", off: "Not configured" })[st] || st}</dd></div>
          ${rows.map(([k, v]) => html`<div><dt>${k}</dt><dd>${v}</dd></div>`)}
        </dl>
        ${f ? html`<p class="small"><b>If it fails:</b> ${f.degrades}</p>`
          : n === "postgres" ? html`<p class="small"><b>If it fails:</b> Nothing can decide money, so readiness fails and the instance leaves the load balancer. It is the one dependency without a fallback, by design.</p>` : ""}
      </div>`);
  }

  async function poll() {
    if (state.stopped || document.hidden) return;
    try { const l = await be.livez(); state.ms = l.ms; } catch { state.ms = null; }
    try { state.ready = await be.readyz(); state.err = null; } catch (e) { state.ready = e.body?.checks ? e.body : null; state.err = e.message; }
    if (be.session.isAdmin) {
      try { state.metrics = await be.metrics(); } catch { /* shown as unknown */ }
      try { state.chaos = await be.chaos.get(); } catch { state.chaos = null; }
    }
    map.setHealth(health());
    panel();
    listeners.forEach(fn => fn(state));
  }

  /** A bid this page placed: pulse its real path, hop time scaled to the measured call. */
  function bid(r) {
    state.lastBid = { status: r.status, timing: r.serverTiming, ms: r.ms };
    const hop = Math.max(70, Math.min(260, (r.ms || 300) / 4));
    map.pulse(PATHS.bid, { hop, tone: r.ok ? "ok" : "bad" });
  }
  function search(backend, ms) {
    state.lastSearch = { backend, ms };
    map.pulse(backend === "postgres" ? PATHS.fallback : PATHS.search, { hop: 110 });
  }

  /** Watch the open lots so every bid anyone places makes a pulse. */
  async function watchLots() {
    try {
      const list = (await be.allAuctions({ status: "ACTIVE" }, 1)).slice(0, watch);
      for (const a of list) {
        subs.push(be.feed.subscribe(a.id, ev => {
          state.events++;
          if (ev.bid) { map.pulse(PATHS.bid.slice(1), { hop: 90 }); setTimeout(() => map.pulse(PATHS.fanout, { hop: 110 }), 320); }
          else map.pulse(PATHS.fanout, { hop: 110 });
        }));
      }
      return list.map(toLot);
    } catch { return []; }
  }

  const timer = setInterval(poll, POLL);
  poll(); panel();
  watchLots();

  return {
    map, state, bid, search, poll,
    onTick: fn => { listeners.add(fn); return () => listeners.delete(fn); },
    stop() { state.stopped = true; clearInterval(timer); subs.forEach(u => u()); map.destroy(); },
  };
}
export { usd };
