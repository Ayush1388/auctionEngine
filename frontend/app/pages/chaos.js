// Chaos lab: break one dependency at a time, keep bidding, and watch what the
// system does. The switches drive /v1/admin/chaos on the real API (started with
// CHAOS_ENABLED=true) or the modelled faults of the demo engine.
import { initShell, toast } from "../shell.js";
import { mountTourBar } from "../tour.js";
import { operatorGate, openLots, plural } from "../lab.js";
import { mountSystem } from "../sysview.js";
import { FAULTS } from "../faults.js";
import { html, $, $$, usd, dur, timeOnly, uuid } from "../util.js";

const be = await initShell({ bar: true, footer: true, nav: "" });
mountTourBar("resilience");
const body = $("#chaosBody");
const sim = be.kind === "sim";
if (!operatorGate(be, body, { next: "chaos.html", what: "Breaking dependencies is an operator action." })) throw new Error("operators only");

const SEARCH = "mustng";                       // a typo on purpose: Elasticsearch forgives it, PostgreSQL does not
const st = { chaos: null, err: null, busy: null, lots: [], lotId: "", feed: [], auto: false, stats: newStats(), fallbackAt: null };
function newStats() { return { bids: { ok: 0, fast503: 0, slow503: 0, other: 0 }, bidMs: [], search: { elasticsearch: 0, postgres: 0, demo: 0 }, searchHits: { es: null, pg: null }, since: Date.now() }; }
let sys, autoTimer, probeTimer;

async function loadChaos() {
  try { st.chaos = await be.chaos.get(); st.err = null; }
  catch (e) { st.chaos = null; st.err = e.status === 404 ? "This API does not have the chaos endpoints. Rebuild it from this branch." : e.message; }
}
const active = () => new Set((st.chaos?.faults || []).filter(f => f.active).map(f => f.name));

/* ------------------------------------------------------------------ page */
function shell() {
  const c = st.chaos;
  return html`
    ${sim ? html`<p class="banner">Simulated. The demo engine models these faults on the same rules as the Go services: breakers open after five failures and recover after ten seconds. For the real thing, start the API with <code>CHAOS_ENABLED=true</code>.</p>`
      : c && !c.enabled ? html`<p class="banner warn">Chaos is switched off on this API. Restart it with <code>CHAOS_ENABLED=true</code> and the switches below start working. Nothing is broken now.</p>` : ""}
    ${st.err ? html`<p class="form-error" role="alert">${st.err}</p>` : ""}
    <div class="chaos-grid">
      <section aria-labelledby="swH"><h2 class="h2" id="swH">Dependencies</h2>
        <ul class="faults" id="faults"></ul>
        <div class="lab-controls" style="margin-top:16px"><button class="btn btn-line btn-sm" id="reset">Restore everything</button><span class="small muted" id="slowNote"></span></div>
      </section>
      <section aria-labelledby="mapH"><h2 class="visually" id="mapH">What a bid touches</h2>
        <div id="map"></div><div id="nodePanel"></div>
      </section>
    </div>

    <section class="ops-sec" aria-labelledby="prH">
      <h2 class="h2" id="prH">Keep bidding while it is broken</h2>
      <div class="lab-controls">
        <label class="field"><span class="field-label">Lot</span><select id="lot">${st.lots.map(l => html`<option value="${l.id}" ${l.id === st.lotId ? "selected" : ""}>${l.lot.title}</option>`)}</select></label>
        <button class="btn" id="bid" ${be.session.id && st.lots.length ? "" : "disabled"}>Place a bid</button>
        <label class="check"><input type="checkbox" id="auto" ${st.auto ? "checked" : ""}> Bid every 3 seconds</label>
      </div>
      <p class="small muted">Each bid is the minimum, so your holds stay small. A search for <b>${SEARCH}</b> runs every 3 seconds in the background.</p>
    </section>

    <div class="split-2">
      <section class="ops-sec" aria-labelledby="obH"><h2 class="h2" id="obH">What the system did</h2><ul class="observed" id="observed" aria-live="polite"></ul></section>
      <section class="ops-sec" aria-labelledby="fdH"><h2 class="h2" id="fdH">Requests</h2><ol class="feed" id="feed"></ol></section>
    </div>`;
}

function paintFaults() {
  const on = active(), c = st.chaos, can = !!(c && (c.enabled || sim));
  $("#faults").innerHTML = String(html`${FAULTS.map(f => html`
    <li class="fault ${on.has(f.name) ? "is-on" : ""}">
      <h3>${f.label}</h3>
      <p>${f.short}. ${on.has(f.name) ? f.degrades : f.does + " Switch it on to see."}</p>
      <button class="switch" role="switch" aria-checked="${on.has(f.name)}" aria-label="Break ${f.label}" data-fault="${f.name}" ${can && st.busy !== f.name ? "" : "disabled"}></button>
    </li>`)}`);
  $$("[data-fault]").forEach(b => b.addEventListener("click", () => toggle(b.dataset.fault, b.getAttribute("aria-checked") !== "true")));
  $("#slowNote").innerHTML = on.has("postgres_slow") ? `Each statement is delayed ${c?.slow_query_ms ?? 40} ms.` : "";
}

async function toggle(name, value) {
  st.busy = name; paintFaults();
  try {
    st.chaos = await be.chaos.set(name, value);
    if (value && name === "elasticsearch") st.fallbackAt = Date.now();
    toast(`${FAULTS.find(f => f.name === name).label} ${value ? "broken" : "restored"}`);
    sys?.poll();
  } catch (e) { st.err = e.message; toast(e.message); }
  st.busy = null; paintFaults(); observe();
}

/* ------------------------------------------------------------------ probes */
function logLine(kind, status, detail, ms, tone) {
  st.feed.unshift({ at: new Date().toISOString(), kind, status, detail, ms, tone });
  if (st.feed.length > 40) st.feed.pop();
  $("#feed").innerHTML = String(html`${st.feed.map(l => html`<li><time>${timeOnly(l.at)}</time><span class="kind">${l.kind}</span><span class="tag ${l.tone}">${l.status}</span><span>${l.detail}</span><span class="r tnum">${dur(l.ms)}</span></li>`)}`);
}

async function bidProbe() {
  if (!be.session.id || !st.lotId) return;
  try {
    const a = await be.getAuction(st.lotId);
    const min = a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment;
    const amount = a.current_bidder_id === be.session.id ? a.current_bid + a.min_increment : min;
    const r = await be.placeBid(st.lotId, amount, { key: uuid() });
    sys.bid(r);
    const b = st.stats.bids;
    st.stats.bidMs.push(r.ms); if (st.stats.bidMs.length > 30) st.stats.bidMs.shift();
    if (r.ok) b.ok++;
    else if (r.status === 503) (r.ms < 60 ? b.fast503++ : b.slow503++);
    else b.other++;
    logLine("Bid", r.status, r.ok ? `placed ${usd(amount)}` : (r.error?.message || "refused"), r.ms, r.ok ? "tag-ok" : r.status === 503 ? "tag-bad" : "tag-warn");
  } catch (e) { logLine("Bid", "ERR", e.message, 0, "tag-bad"); }
  observe();
}

async function searchProbe() {
  const t0 = performance.now();
  try {
    const p = await be.search(SEARCH, { status: "ACTIVE", limit: 10 });
    const ms = performance.now() - t0, backend = p.backend === "demo" ? "elasticsearch" : p.backend;
    st.stats.search[backend] = (st.stats.search[backend] || 0) + 1;
    st.stats.searchHits[backend === "postgres" ? "pg" : "es"] = p.auctions.length;
    sys.search(backend, ms);
    logLine("Search", 200, `${p.auctions.length} results from ${backend === "postgres" ? "PostgreSQL" : "Elasticsearch"}`, ms, backend === "postgres" ? "tag-warn" : "tag-ok");
  } catch (e) { logLine("Search", e.status || "ERR", e.message, performance.now() - t0, "tag-bad"); }
  observe();
}

/* ------------------------------------------------------------------ narration */
function observe() {
  const el = $("#observed"); if (!el) return;
  const on = active(), s = st.stats, out = [];
  const med = a => { const v = [...a].sort((x, y) => x - y); return v.length ? v[Math.floor(v.length / 2)] : null; };
  const brState = name => { const x = sys?.state.metrics?.families.find(f => f.name === "auction_circuit_breaker_state")?.samples.find(y => y.labels?.name === name); return x ? x.value : null; };
  if (!on.size) out.push(["ok", html`<b>Everything is healthy.</b> Switch a dependency off and keep bidding. ${s.bids.ok ? html`So far ${plural(s.bids.ok, "bid")} placed, median ${dur(med(s.bidMs))}.` : ""}`]);
  if (on.has("elasticsearch")) out.push([s.search.postgres ? "warn" : "ok", s.search.postgres
    ? html`<b>Search fell back to PostgreSQL.</b> ${plural(s.search.postgres, "search", "searches")} answered there. The typo in "${SEARCH}" now finds ${s.searchHits.pg ?? 0} cars instead of ${s.searchHits.es ?? "several"}: PostgreSQL full-text search has no typo tolerance. Breaker: ${({ 0: "closed", 1: "half open", 2: "open" })[brState("elasticsearch")] ?? "unknown"}.`
    : html`<b>Elasticsearch is failing.</b> Waiting for the next search to show the fallback.`]);
  if (on.has("bidding")) out.push([s.bids.fast503 ? "ok" : "warn", s.bids.fast503
    ? html`<b>The breaker is open.</b> ${plural(s.bids.fast503, "bid")} refused instantly with 503, after ${plural(s.bids.slow503, "slow failure")}. Nobody waits for a timeout and nothing was charged.`
    : html`<b>The bidding service is down.</b> Place a bid: the first few fail slowly, then the breaker opens and the rest fail at once.`]);
  if (on.has("postgres_slow")) out.push(["warn", html`<b>PostgreSQL is slow.</b> Median bid time is ${s.bidMs.length ? dur(med(s.bidMs)) : "waiting for a bid"}. Bids still commit one at a time and the books still balance; they just hold the row lock longer.`]);
  if (on.has("redis")) out.push(["warn", html`<b>Redis is down.</b> The cache is bypassed, trending comes from PostgreSQL and rate limits fail open. Bids and live updates on this instance are unaffected, because nothing that decides money reads from Redis.`]);
  if (on.has("kafka")) {
    const pend = sys?.state.metrics?.families.find(f => f.name === "auction_outbox_pending")?.samples[0]?.value;
    out.push(["warn", html`<b>Kafka is unreachable.</b> Synchronous bids are unaffected. ${pend != null ? html`${plural(pend, "event")} waiting in the outbox` : "Events wait in the outbox"}, in order, and flow when it returns.`]);
  }
  el.innerHTML = String(html`${out.map(([tone, h]) => html`<li class="${tone}">${h}</li>`)}`);
}

/* ------------------------------------------------------------------ boot */
await loadChaos();
st.lots = (await openLots(be).catch(() => [])).sort((a, b) => a.lot.minNext - b.lot.minNext);
st.lotId = st.lots[0]?.id || "";
body.setAttribute("aria-busy", "false");
body.innerHTML = String(shell());
sys = mountSystem(be, { mapRoot: $("#map"), panelRoot: $("#nodePanel") });
sys.onTick(async () => { await loadChaos(); paintFaults(); observe(); });
paintFaults(); observe();
$("#reset").addEventListener("click", async () => { try { st.chaos = await be.chaos.reset(); st.stats = newStats(); toast("Everything restored"); } catch (e) { toast(e.message); } paintFaults(); observe(); sys.poll(); });
$("#lot").addEventListener("change", e => { st.lotId = e.target.value; });
$("#bid").addEventListener("click", bidProbe);
$("#auto").addEventListener("change", e => {
  st.auto = e.target.checked; clearInterval(autoTimer);
  if (st.auto) { bidProbe(); autoTimer = setInterval(bidProbe, 3000); }
});
probeTimer = setInterval(() => { if (!document.hidden) searchProbe(); }, 3000);
searchProbe();
addEventListener("pagehide", () => { clearInterval(autoTimer); clearInterval(probeTimer); sys.stop(); });
