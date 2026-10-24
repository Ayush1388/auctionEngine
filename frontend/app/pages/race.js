// Race theater: several bids on one price, and the row lock that decides who goes first.
//  - "Race for real" fires the bids at the backend and draws each one's lock wait and work
//    from the Server-Timing it answered with (simulated timings in the demo engine).
//  - "Model" replays the same race under each locking strategy so the difference is visible.
//  - The numbers in the charts are the ones measured in docs/PERFORMANCE.md.
import { initShell } from "../shell.js";
import { mountTourBar } from "../tour.js";
import { openLots, plural } from "../lab.js";
import { html, $, $$, usd, dur, uuid } from "../util.js";

const be = await initShell({ bar: true, footer: true, nav: "" });
mountTourBar("decision");
const body = $("#raceBody");
const sim = be.kind === "sim";

// docs/PERFORMANCE.md: Run 2 (pessimistic, pool of 20) and Run 3 (BID_LOCKING=optimistic), 32 workers, 20 s
const MEASURED = {
  pessimistic: { label: "Pessimistic", p99: 322.2, p50: 49.5, accepted: 4572, rejected: 2551, benchBids: 255, benchAccepted: 178 },
  optimistic: { label: "Optimistic", p99: 159.5, p50: 56.6, accepted: 4509, rejected: 4191, benchBids: 682, benchAccepted: 178 },
};

const st = { strategy: "pessimistic", n: 8, lots: [], lotId: "", running: false, real: null, err: null };

/* ------------------------------------------------------------------ the model */
// Work per bid is taken from a real measurement when there is one (write + commit), else a typical 3.4 ms.
function model(strategy, n, work = 3.4) {
  const out = [];
  if (strategy === "pessimistic") {
    // the row lock is a queue: each bid waits for every bid ahead of it, including the rejected ones
    let t = 0;
    for (let i = 0; i < n; i++) {
      const w = i === 0 ? work : 0.35;               // a refused bid only reads and decides
      out.push({ who: i === 0 ? "Winner" : `Bid ${i + 1}`, wait: t, work: w, ok: i === 0, note: i === 0 ? "accepted" : "too low" });
      t += w;
    }
  } else {
    // nobody waits: everyone reads, decides, then races to write. One compare-and-swap wins; the rest retry once with backoff
    for (let i = 0; i < n; i++) {
      const read = 0.4;
      if (i === 0) out.push({ who: "Winner", wait: 0, work: read + work, ok: true, note: "accepted" });
      else out.push({ who: `Bid ${i + 1}`, wait: read + work * 0.6, work: 1 + (i % 3) + 0.4, ok: false, note: "conflict, then too low" });
    }
  }
  return out;
}

/* ------------------------------------------------------------------ real race */
async function raceForReal() {
  const lot = st.lots.find(l => l.id === st.lotId);
  if (!lot || st.running || !be.session.id) return;
  st.running = true; st.err = null; paint();
  try {
    const raw = await be.getAuction(lot.id);
    const amount = raw.current_bid == null ? Math.max(raw.starting_price, 1) : raw.current_bid + raw.min_increment;
    // distinct bidders where the demo can make them; the same account on the real API (the second copy of a price is refused)
    const actors = [];
    for (let i = 0; i < st.n; i++) actors.push(sim && be.trust && i > 0 ? await be.trust.attacker(amount * 3) : null);
    if (sim) await be.chaos.setRateLimits(false);
    const t0 = performance.now();
    const answers = await Promise.all(actors.map((a, i) => (a ? a.placeBid(lot.id, amount, uuid()) : be.placeBid(lot.id, amount, { key: uuid() }))));
    const total = performance.now() - t0;
    if (sim) await be.chaos.setRateLimits(true);
    st.real = { amount, total, rows: answers.map((r, i) => ({ who: i === 0 ? "You" : `Rival ${i}`, ok: r.ok, status: r.status, ms: r.ms, tm: r.serverTiming, minimum: r.data?.minimum_amount })) };
  } catch (e) { st.err = e.message; try { if (sim) await be.chaos.setRateLimits(true); } catch { /* ignore */ } }
  st.running = false; paint();
}

/* ------------------------------------------------------------------ drawing */
function lanesHTML(rows, scale, label) {
  return html`<div class="race-lanes" role="img" aria-label="${label}">
    ${rows.map(r => html`<div class="race-lane">
      <span class="who">${r.who}</span>
      <div class="race-track">
        ${r.wait > 0.01 ? html`<span class="race-seg wait" style="left:0;width:${Math.max(0.8, r.wait / scale * 100)}%" title="waiting for the lock ${dur(r.wait)}"></span>` : ""}
        <span class="race-seg ${r.ok ? "work" : "lost"}" style="left:${r.wait / scale * 100}%;width:${Math.max(1.2, r.work / scale * 100)}%">${r.ok ? "wins" : ""}</span>
      </div>
      <span class="ms tnum small">${dur(r.wait + r.work)}</span>
    </div>`)}
  </div>`;
}

function realRows() {
  const r = st.real; if (!r) return [];
  return r.rows.map(x => {
    const lock = x.tm?.lock ?? 0, work = x.tm ? (x.tm.decide || 0) + (x.tm.write || 0) + (x.tm.commit || 0) : x.ms;
    return { who: x.who, wait: lock, work, ok: x.ok };
  });
}

function bars(rows) {
  const max = Math.max(...rows.flatMap(r => [r.a, r.b]));
  return html`<div class="bars">${rows.map(r => html`
    <div class="bar-group"><span class="small">${r.label}</span>
      <div class="bar"><i style="width:${r.a / max * 100}%"></i><b class="tnum">${r.fmt(r.a)}</b><span class="small muted">pessimistic</span></div>
      <div class="bar alt"><i style="width:${r.b / max * 100}%"></i><b class="tnum">${r.fmt(r.b)}</b><span class="small muted">optimistic</span></div></div>`)}</div>`;
}

function paint() {
  const m = MEASURED, real = realRows();
  const work = st.real ? (st.real.rows.find(x => x.ok)?.tm ? (st.real.rows.find(x => x.ok).tm.write + st.real.rows.find(x => x.ok).tm.commit) : undefined) : undefined;
  const mod = model(st.strategy, st.n, work);
  const scaleM = Math.max(...mod.map(r => r.wait + r.work), 1) * 1.05;
  const scaleR = Math.max(...real.map(r => r.wait + r.work), 1) * 1.05;
  const losers = real.filter(r => !r.ok);
  body.setAttribute("aria-busy", "false");
  body.innerHTML = String(html`
    <div class="race">
      <section aria-labelledby="rrH">
        <h2 class="h2" id="rrH">Race for real</h2>
        <p class="lead">${sim ? "Several accounts bid the same price at the same instant." : "Several copies of one bid are sent at the same instant."} Exactly one is accepted. The rest wait for the lock, then learn the price moved.</p>
        <div class="lab-controls">
          <label class="field"><span class="field-label">Lot</span><select id="lot">${st.lots.map(l => html`<option value="${l.id}" ${l.id === st.lotId ? "selected" : ""}>${l.lot.title}</option>`)}</select></label>
          <label class="field"><span class="field-label">Bidders</span><select id="n">${[2, 4, 8, 16].map(n => html`<option ${n === st.n ? "selected" : ""}>${n}</option>`)}</select></label>
          <button class="btn ${st.running ? "busy" : ""}" id="go" ${st.running || !be.session.id || !st.lots.length ? "disabled" : ""}>Start the race</button>
        </div>
        ${!be.session.id ? html`<p class="form-error">Sign in to race. <a class="linkish" href="account.html?next=race.html">Sign in</a></p>` : ""}
        ${st.err ? html`<p class="form-error" role="alert">${st.err}</p>` : ""}
        ${st.real ? html`
          ${lanesHTML(real, scaleR, "Measured race")}
          <p class="small muted"><span class="tag tag-warn">waiting for the lock</span> <span class="tag tag-ok">working</span> The first bid at ${usd(st.real.amount)} won. ${plural(losers.length, "bid")} waited a median ${dur(losers.length ? losers.map(l => l.wait).sort((a, b) => a - b)[Math.floor(losers.length / 2)] : 0)} behind it. The whole burst took ${dur(st.real.total)}. ${sim ? "Timings are simulated." : "Timings come from the Server-Timing header."}</p>`
          : html`<p class="small muted">Press start. Each bar is one bid: the amber part is time spent waiting for the auction's row lock, the red part is its own transaction.</p>`}
      </section>

      <section aria-labelledby="mdH">
        <h2 class="h2" id="mdH">Same race, two strategies</h2>
        <div class="presets" role="group" aria-label="Locking strategy">
          ${Object.entries(m).map(([k, v]) => html`<button class="preset" data-s="${k}" aria-pressed="${st.strategy === k}">${v.label}</button>`)}
        </div>
        ${lanesHTML(mod, scaleM, `Model of ${st.n} bids under ${st.strategy} locking`)}
        <p class="small muted">${st.strategy === "pessimistic"
          ? "Pessimistic: SELECT ... FOR UPDATE makes bids queue on the auction row. Each waits for everyone ahead, then sees the price the winner set. Predictable, but a hot lot builds a queue."
          : "Optimistic: nobody waits. Everyone reads and decides, then writes with a version check. One wins; the rest retry with backoff, find the price moved, and are refused. No queue, but wasted work."}
          This is a model, drawn to scale from a typical ${dur(work ?? 3.4)} write.</p>
      </section>
    </div>

    <section class="ops-sec" aria-labelledby="pfH" style="margin-top:clamp(28px,4vw,52px)">
      <h2 class="h2" id="pfH">What it cost, measured</h2>
      <p class="lead">From docs/PERFORMANCE.md: 32 closed-loop workers for 20 seconds, half the bids aimed at one hot lot, on two shared vCPUs.</p>
      ${bars([
        { label: "Bid latency, p99 (ms)", a: m.pessimistic.p99, b: m.optimistic.p99, fmt: v => v.toFixed(0) + " ms" },
        { label: "Bids accepted in 20 s", a: m.pessimistic.accepted, b: m.optimistic.accepted, fmt: v => v.toLocaleString() },
        { label: "Bid attempts per second, hot lot (Go benchmark)", a: m.pessimistic.benchBids, b: m.optimistic.benchBids, fmt: v => v + " /s" },
        { label: "Accepted bids per second, hot lot (Go benchmark)", a: m.pessimistic.benchAccepted, b: m.optimistic.benchAccepted, fmt: v => "about " + v + " /s" },
      ])}
      <p class="small muted">The finding: optimistic locking halves the tail latency, but accepted bids do not move. On one hot lot the auction itself, one price changing at a time, sets how many bids can be accepted. The strategy only decides whether the excess waits or is refused quickly.</p>
    </section>`);
  $$("[data-s]", body).forEach(b => b.addEventListener("click", () => { st.strategy = b.dataset.s; paint(); }));
  $("#lot")?.addEventListener("change", e => { st.lotId = e.target.value; });
  $("#n")?.addEventListener("change", e => { st.n = +e.target.value; paint(); });
  $("#go")?.addEventListener("click", raceForReal);
}

st.lots = (await openLots(be).catch(() => [])).sort((a, b) => a.lot.minNext - b.lot.minNext);
st.lotId = st.lots[0]?.id || "";
paint();
