// System status. Probes are public; metrics are the operator's (GET /v1/admin/metrics).
import { initShell } from "../shell.js";
import { lineChart, chartTable, quantile } from "../chart.js";
import { html, raw, esc, $, $$, dur, usd } from "../util.js";

const be = await initShell({ bar: true, footer: true, nav: "" });
const body = $("#statusBody");
const POLL = 2000, KEEP = 150;
const WINDOWS = [["60", "1 minute"], ["180", "3 minutes"]];
let windowSec = 60, paused = false, ready = null, live = null, snaps = [], series = { bps: [], p95: [], lag: [] }, err = null, tableOpen = {};

const fam = (s, name) => s?.families.find(f => f.name === name);
const sum = (f, pick = () => true) => (f?.samples || []).filter(x => pick(x.labels || {})).reduce((a, x) => a + (x.value ?? 0), 0);
const gauge = (s, name) => sum(fam(s, name));

/** Turn two snapshots into the numbers on screen. */
function derive(prev, cur) {
  const dt = (Date.parse(cur.time) - Date.parse(prev.time)) / 1000 || POLL / 1000;
  const acc = s => sum(fam(s, "auction_bids_total"), l => l.result === "accepted");
  const bps = Math.max(0, (acc(cur) - acc(prev)) / dt);
  // latency over the last few polls: the difference of two cumulative histograms is that window's histogram
  const h = s => fam(s, "auction_bid_duration_seconds")?.samples[0];
  const base = snaps[Math.max(0, snaps.length - 6)] || prev, a = h(cur), b = h(base);
  let p95 = null;
  if (a && b) { const total = (a.count ?? 0) - (b.count ?? 0); if (total > 0) p95 = quantile(a.buckets.map((x, i) => ({ le: x.le, count: x.count - (b.buckets[i]?.count ?? 0) })), total, 0.95) * 1000; }
  return { t: Date.parse(cur.time), bps, p95 };
}

async function poll() {
  if (paused || document.hidden) return;
  try { ready = await be.readyz(); err = null; } catch (e) { ready = null; err = e.message; }
  let live2 = null; try { live2 = await be.livez(); } catch { /* shown as down */ }
  live = live2;
  if (be.session.isAdmin) {
    try {
      const cur = await be.metrics();
      if (snaps.length) {
        const d = derive(snaps.at(-1), cur);
        series.bps.push({ t: d.t, v: d.bps });
        series.p95.push({ t: d.t, v: d.p95 ?? series.p95.at(-1)?.v ?? 0 });
        series.lag.push({ t: d.t, v: gauge(cur, "auction_kafka_consumer_lag") });
        for (const k of Object.keys(series)) if (series[k].length > KEEP) series[k].shift();
      }
      snaps.push(cur); if (snaps.length > 12) snaps.shift();
    } catch (e) { err = e.message; }
  }
  if (!body.querySelector(".chart:hover, .chart:focus-within, .tview[open]")) draw();   // never redraw under someone reading a tooltip or a table
}

const STATE = { 0: ["Closed", "ok"], 1: ["Half open", "warn"], 2: ["Open", "bad"] };
const win = pts => pts.filter(p => p.t >= (pts.at(-1)?.t ?? 0) - windowSec * 1000);
const check = (name, ok, detail) => html`<li class="probe"><span class="state ${ok ? "ok" : "bad"}">${ok ? "OK" : "Down"}</span><b>${name}</b><span class="muted small">${detail}</span></li>`;

function probes() {
  const checks = ready?.checks || {};
  return html`
    <section class="ops-sec" aria-labelledby="hH"><h2 class="h2" id="hH">Health checks</h2>
      <ul class="probes">
        ${check("Liveness", !!live, live ? `process is up, answered in ${dur(live.ms)}` : "no answer")}
        ${check("Readiness", ready?.status === "ok", ready ? `status: ${ready.status}` : err || "no answer")}
        ${Object.entries(checks).map(([k, v]) => check(k.charAt(0).toUpperCase() + k.slice(1), v === "ok", v === "ok" ? "reachable" : "failing"))}
      </ul>
      <p class="small muted">${be.kind === "sim" ? "Demo engine: these checks are simulated." : "Readiness drains before a shutdown, so a rolling deploy sheds no requests. Optional dependencies are reported but do not fail it."}</p>
    </section>`;
}

function operatorHTML() {
  const cur = snaps.at(-1);
  if (!cur) return html`<div class="skel" style="height:240px"></div>`;
  const bps = win(series.bps), p95 = win(series.p95), lag = win(series.lag);
  const last = a => a.at(-1)?.v ?? 0;
  const results = (fam(cur, "auction_bids_total")?.samples || []).map(x => [x.labels.result, x.value]).sort((a, b) => b[1] - a[1]);
  const breakers = (fam(cur, "auction_circuit_breaker_state")?.samples || []).map(x => [x.labels.name, x.value]);
  const tile = (label, value, note = "") => html`<div class="tile"><span class="lab mono">${label}</span><b class="tnum">${value}</b>${note ? html`<span class="small muted">${note}</span>` : ""}</div>`;
  const dead = gauge(cur, "auction_outbox_dead_lettered");
  return html`
    <div class="ops-filters" role="group" aria-label="Chart window"><span class="mono lab">Window</span>
      ${WINDOWS.map(([k, l]) => html`<button class="preset" data-win="${k}" aria-pressed="${String(windowSec) === k}">${l}</button>`)}
      <button class="preset" id="pause" aria-pressed="${paused}">${paused ? "Resume" : "Pause"}</button>
      <span class="small muted">Updates every 2 seconds. ${snaps.length < 2 ? "Collecting the first readings." : ""}</span></div>

    <section class="ops-hero" aria-label="Throughput">
      <div><span class="mono lab">Bids accepted per second</span><b class="hero tnum">${last(bps).toFixed(1)}</b>
        <p class="small muted">Place a bid, or run <a href="lot.html?id=mustang#stress">Stress it</a> on a lot, and watch this move.</p></div>
      <div class="chart-card"><div data-chart="bps"></div>${this_table("bps", bps, v => v.toFixed(1), "Bids per second")}</div>
    </section>

    <section class="ops-two">
      <div class="chart-card"><h2 class="h3">Bid latency, p95</h2><p class="small muted">95 percent of bids finish faster than this, including the wait for the auction's row lock.</p><div data-chart="p95"></div>${this_table("p95", p95, v => dur(v), "p95 latency")}</div>
      <div class="chart-card"><h2 class="h3">Kafka consumer lag</h2><p class="small muted">Records waiting for a bid worker. It rises during a burst and returns to zero.</p><div data-chart="lag"></div>${this_table("lag", lag, v => String(Math.round(v)), "Records waiting")}</div>
    </section>

    <section class="ops-sec" aria-label="Queues and connections"><h2 class="h2">Queues and connections</h2>
      <div class="tiles">
        ${tile("Outbox waiting", gauge(cur, "auction_outbox_pending"), "events committed, not yet delivered")}
        ${tile("Dead-lettered", dead, dead ? "needs the operator console" : "nothing parked")}
        ${tile("WebSocket connections", gauge(cur, "auction_ws_connections"), "people watching live")}
        ${tile("Database connections", `${gauge(cur, "auction_db_pool_acquired_connections")} / ${gauge(cur, "auction_db_pool_max_connections")}`, "in use / pool size")}
        ${tile("Latency now (p95)", last(p95) ? dur(last(p95)) : "-", "last few seconds")}
      </div>
    </section>

    <section class="ops-two">
      <div><h2 class="h2">Bid outcomes</h2>
        ${results.length ? html`<table class="tbl"><thead><tr><th>Outcome</th><th class="r">Count</th></tr></thead><tbody>${results.map(([k, v]) => html`<tr><td>${k.replace(/_/g, " ")}</td><td class="r tnum">${v}</td></tr>`)}</tbody></table>` : html`<p class="muted">No bids since the process started.</p>`}</div>
      <div><h2 class="h2">Circuit breakers</h2>
        <table class="tbl"><thead><tr><th>Dependency</th><th>State</th></tr></thead><tbody>${breakers.map(([k, v]) => html`<tr><td>${k}</td><td><span class="state ${STATE[v][1]}">${STATE[v][0]}</span></td></tr>`)}</tbody></table>
        <p class="small muted">When a dependency fails repeatedly its breaker opens: search falls back to PostgreSQL, rate limits fail open, and bids return a fast 503 instead of hanging.</p></div>
    </section>`;
}

/** The numbers behind a chart, for anyone who would rather read than hover. */
function this_table(key, pts, fmt, label) {
  return html`<details class="tview" data-tv="${key}" ${tableOpen[key] ? "open" : ""}><summary class="linkish">Show as a table</summary><div data-table="${key}"></div></details>`;
}

function draw() {
  body.setAttribute("aria-busy", "false");
  const admin = be.session.isAdmin;
  body.innerHTML = String(html`
    ${probes()}
    ${admin ? operatorHTML() : html`
      <section class="ops-sec"><h2 class="h2">Live metrics</h2>
        <div class="empty"><h3>Metrics are for operators</h3><p>${be.session.id ? "This account is not an operator." : "Sign in as the operator account to see bids per second, latency and queue depth."}</p>
        <a class="btn" href="account.html?next=status.html">${be.session.id ? "Switch account" : "Sign in"}</a></div></section>`}
    ${err && admin ? html`<p class="form-error" role="alert">${err}</p>` : ""}`);
  if (!admin || !snaps.length) return;
  const bps = win(series.bps), p95 = win(series.p95), lag = win(series.lag);
  const defs = { bps: [bps, v => v.toFixed(1), "Bids per second"], p95: [p95, v => dur(v), "p95 latency"], lag: [lag, v => String(Math.round(v)), "Records waiting"] };
  for (const [k, [pts, fmt, label]] of Object.entries(defs)) {
    const root = $(`[data-chart="${k}"]`); if (!root) continue;
    lineChart(root, { title: label, points: pts, format: fmt, axisFormat: k === "p95" ? (v => (v >= 1000 ? (v / 1000).toFixed(1) + " s" : Math.round(v) + " ms")) : (v => (v >= 10 ? String(Math.round(v)) : v.toFixed(1))), min: k === "p95" ? 5 : k === "lag" ? 4 : 1 });
    const holder = $(`[data-table="${k}"]`); if (holder) holder.append(chartTable(pts, fmt, label));
  }
  $$("[data-win]", body).forEach(b => b.addEventListener("click", () => { windowSec = +b.dataset.win; draw(); }));
  $("#pause")?.addEventListener("click", () => { paused = !paused; draw(); });
  $$("[data-tv]", body).forEach(d => d.addEventListener("toggle", () => { tableOpen[d.dataset.tv] = d.open; }));
}

draw();
poll();
setInterval(poll, POLL);
addEventListener("resize", () => { clearTimeout(draw.t); draw.t = setTimeout(draw, 200); });
