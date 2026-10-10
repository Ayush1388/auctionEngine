// "Behind the bid": follow one bid from the click to everyone's screen.
//
// Nothing here is invented. Each row says where its number comes from:
//   measured            the browser's own clock
//   server timing       the Server-Timing header the API sends with the bid
//   event timestamps    timestamps on the WebSocket message (both from the API's clock)
//   simulated           the demo engine, which models these phases
// A step that cannot be seen from the browser is listed as such, not guessed.
import { html, raw, esc, $, $$, dur, usd, timeOnly, short, reduceMotion } from "./util.js";
import { bidderLabel } from "./bidding.js";
import { config } from "./config.js";

const PHASES = [
  ["lock", "Row lock", "Waiting for SELECT ... FOR UPDATE on the auction. Bids on one lot queue here, one at a time."],
  ["decide", "Rules", "Minimum bid, funds, owner and end time, checked on data nobody else can change."],
  ["write", "Write", "Bid, wallet hold, two ledger journals, the auction row and the outbox event, in one transaction."],
  ["commit", "Commit", "COMMIT. After this the bid is durable."],
];

/** The rows of a bid's timeline, in two groups so each can be read on its own scale. */
export function hopsFor(at, be) {
  const sim = be.kind === "sim", S = live => (sim ? "simulated" : live);
  const t = at.timing, svc = t?.bidding;
  const journey = [], inside = [];
  if (at.rtt != null) {
    if (svc != null) {
      journey.push({ id: "net", label: "Network and gateway", desc: "Rate limit, token check, validation and the wire.", ms: Math.max(0, at.rtt - svc), src: S("browser minus server"), fam: "net" });
      journey.push({ id: "svc", label: "Bidding transaction", desc: "Everything the bidding service did, shown in detail below.", ms: svc, src: S("server timing"), fam: "db" });
    } else journey.push({ id: "rtt", label: "Round trip", desc: "From the click until the answer came back.", ms: at.rtt, src: S("measured"), fam: "net" });
  }
  if (t && t.lock != null) {
    let sum = 0;
    for (const [k, label, desc] of PHASES) if (t[k] != null && (t[k] > 0 || at.ok)) { inside.push({ id: k, label, desc, ms: t[k], src: S("server timing"), fam: "db" }); sum += t[k]; }
    if (svc != null && svc - sum > 0.05) inside.push({ id: "oh", label: "Service overhead", desc: "The call outside the transaction (a gRPC hop when the bidding service runs separately).", ms: svc - sum, src: S("server timing"), fam: "db" });
  }
  if (at.ws?.outbox != null) journey.push({ id: "outbox", label: "Outbox to fan-out", desc: "The event leaves the outbox and the new snapshot is published to every API instance.", ms: at.ws.outbox, src: S("event timestamps"), fam: "ev" });
  if (at.ws) journey.push({ id: "e2e", label: "Click to live update", desc: "From the click until this browser received the WebSocket message.", ms: at.ws.e2e, src: S("browser clock"), fam: "total" });
  return { journey, inside, rows: [...journey, ...inside] };
}

/**
 * The timeline of one bid as a waterfall: each row is a span with a start offset and a
 * duration, drawn on one shared time axis. Offsets are reconstructed from what the browser
 * can see (round trip, Server-Timing, event timestamps); with a Jaeger address configured the
 * real spans of the trace are fetched and drawn instead.
 */
export function waterfallRows(at) {
  const rows = [];
  if (at.rtt == null) return rows;
  const svc = at.timing?.bidding, t = at.timing || {};
  const net = svc != null ? Math.max(0, at.rtt - svc) / 2 : 0;
  rows.push({ d: 0, name: "POST /bids", start: 0, dur: at.rtt, fam: "" });
  if (svc != null) {
    rows.push({ d: 1, name: "Network and gateway", start: 0, dur: net, fam: "net" });
    rows.push({ d: 1, name: "Bidding service", start: net, dur: svc, fam: "" });
    let c = net;
    for (const k of ["lock", "decide", "write", "commit"]) if (t[k] != null) { rows.push({ d: 2, name: ({ lock: "Row lock", decide: "Rules", write: "Write", commit: "Commit" })[k], start: c, dur: t[k], fam: "" }); c += t[k]; }
    rows.push({ d: 1, name: "Network back", start: net + svc, dur: net, fam: "net" });
  }
  if (at.ws) {
    const commitAt = svc != null ? net + svc : at.rtt;
    if (at.ws.outbox != null) rows.push({ d: 0, name: "Outbox to fan-out", start: commitAt, dur: at.ws.outbox, fam: "ev" });
    rows.push({ d: 0, name: "WebSocket to this screen", start: commitAt + (at.ws.outbox || 0), dur: Math.max(0.1, at.ws.e2e - commitAt - (at.ws.outbox || 0)), fam: "ev" });
  }
  return rows;
}

export class TracePanel {
  constructor(session, be) {
    this.spans = new Map();
    this.s = session; this.be = be; this.sel = null; this.raf = 0; this.opener = null; this.stress = { state: "idle" };
    this.el = document.createElement("aside");
    this.el.className = "trace"; this.el.id = "trace"; this.el.setAttribute("aria-hidden", "true"); this.el.setAttribute("role", "dialog"); this.el.setAttribute("aria-label", "Behind the bid");
    document.body.append(this.el);
    this.el.addEventListener("click", e => this.#click(e));
    document.addEventListener("keydown", e => { if (e.key === "Escape" && this.isOpen) this.close(); });
    for (const ev of ["attempt", "event", "lot", "outcome"]) session.on(ev, () => this.#schedule());
    this.render();
  }

  get isOpen() { return this.el.classList.contains("open"); }
  open(opener) { this.opener = opener || document.activeElement; this.el.classList.add("open"); this.el.setAttribute("aria-hidden", "false"); this.render(); this.el.querySelector(".trace-x")?.focus(); }
  close() { this.el.classList.remove("open"); this.el.setAttribute("aria-hidden", "true"); this.opener?.focus?.(); }
  toggle(opener) { this.isOpen ? this.close() : this.open(opener); }
  #schedule() { if (!this.isOpen || this.raf) return; this.raf = requestAnimationFrame(() => { this.raf = 0; this.render(); }); }

  /* ---------------------------------------------------------------- stress */
  async startStress(bidders, rounds) {
    this.stress = { state: "running", bidders, rounds, round: 0, accepted: 0, rejected: 0 };
    this.render();
    try {
      await this.be.stress.start({ auctionId: this.s.id, bidders, rounds }, ev => {
        if (ev.type === "progress") this.stress = { ...this.stress, ...ev, state: "running" };
        else if (ev.type === "done") this.#finishStress(ev);
        else if (ev.type === "error") this.stress = { ...this.stress, state: "error", error: ev.error };
        this.#schedule();
      });
    } catch (e) { this.stress = { state: "error", error: e.message || "Could not start the run" }; this.render(); }
  }
  async #finishStress(ev) {
    this.stress = { ...this.stress, ...ev, state: "done", checks: ev.invariants?.checks || [] };
    this.render();
    // the page checks the result itself, from what the API returns, not only from what the bots say
    try {
      const h = await this.be.bids(this.s.id, { limit: 100 });
      const a = h.bids, ordered = a.every((b, i) => i === a.length - 1 || b.amount > a[i + 1].amount);
      const own = { name: "Bid history from the API is strictly increasing", ok: ordered, detail: `last ${a.length} bids read back from the server` };
      this.stress.checks = [own, ...this.stress.checks.filter(c => !/strictly increasing/i.test(c.name))];
      if (this.be.session.isAdmin) {
        const r = await this.be.reconcile();
        this.stress.checks.push({ name: "Books reconcile (operator check)", ok: r.ok, detail: `${r.money_in_wallets} in wallets, ${r.reserved_in_wallets} held` });
      }
    } catch { /* the bots' own checks are still shown */ }
    this.s.reload(); this.render();
  }

  #click(e) {
    const b = e.target.closest("[data-act]"); if (!b) return;
    const act = b.dataset.act;
    if (act === "close") this.close();
    else if (act === "pick") { this.sel = b.dataset.id; this.render(); }
    else if (act === "copy") { navigator.clipboard?.writeText(b.dataset.val); b.textContent = "Copied"; setTimeout(() => this.render(), 900); }
    else if (act === "stress") { if (this.stress.state !== "running") this.startStress(+$("#stN", this.el).value, +$("#stR", this.el).value); }
    else if (act === "ff") this.be.fastForward(this.s.id, 90).catch(() => {});
  }

  /* ---------------------------------------------------------------- drawing */
  render() {
    const s = this.s, be = this.be, sim = be.kind === "sim";
    const at = s.attempts.find(a => a.id === this.sel) || s.attempts[0];
    const keep = this.el.querySelector(".trace-body")?.scrollTop || 0;
    this.el.innerHTML = String(html`
      <header class="trace-head">
        <div><h2>Behind the bid</h2><p class="mono">${sim ? "Demo engine. Timings are simulated." : "Live backend. Timings are measured."}</p></div>
        <button class="trace-x" data-act="close" aria-label="Close">×</button>
      </header>
      <div class="trace-body">
        <p class="trace-intro">Place a bid, then watch it travel: the browser, the API, the bidding transaction in PostgreSQL, the outbox, and the WebSocket back to every screen. Each number says where it comes from.</p>
        ${raw(at ? this.#attempt(at) : `<div class="trace-empty"><h3>No bid yet</h3><p>Your next bid appears here with its timeline.</p></div>`)}
        ${raw(this.#attempts())}
        ${raw(this.#events())}
        ${raw(this.#stressHTML())}
        ${raw(sim ? this.#demoControls() : "")}
      </div>`);
    const body = this.el.querySelector(".trace-body"); if (body) body.scrollTop = keep;
  }

  #hop(r, max) {
    const w = r.fam === "total" ? 100 : Math.max(1.2, r.ms / max * 100).toFixed(1);
    return `<li class="hop hop-${r.fam}" title="${esc(r.desc)}">
      <div class="hop-name"><b>${esc(r.label)}</b><span>${esc(r.desc)}</span></div>
      <div class="hop-val"><b class="tnum">${esc(dur(r.ms))}</b><span class="mono">${esc(r.src)}</span></div>
      <div class="hop-bar" aria-hidden="true"><i style="width:${w}%"></i></div></li>`;
  }

  #attempt(at) {
    const be = this.be, { journey, inside } = hopsFor(at, be);
    const e2e = journey.find(r => r.id === "e2e");
    const jmax = Math.max(1, ...journey.map(r => r.ms)), imax = Math.max(0.001, ...inside.map(r => r.ms));
    const state = at.state === "sent" ? "In flight" : at.ok ? (at.replayed ? "Replayed, no new bid" : `${at.status} ${at.status === 201 ? "Created" : "OK"}`) : `${at.status || "No answer"} ${at.error?.message || ""}`;
    return `
      <section class="tr-sec" aria-labelledby="trH">
        <div class="tr-sechead"><h3 id="trH">Bid ${usd(at.amount)}</h3><span class="tag ${at.state === "sent" ? "tag-warn" : at.ok ? "tag-ok" : "tag-bad"}">${esc(state)}</span></div>
        ${e2e ? `<p class="tr-big"><b class="tnum">${esc(dur(e2e.ms))}</b><span>from your click to the live update on screen</span></p>`
          : at.rtt != null ? `<p class="tr-big"><b class="tnum">${esc(dur(at.rtt))}</b><span>round trip${at.ok && !at.replayed ? "; waiting for the live update" : ""}</span></p>` : `<p class="tr-big"><b class="tnum">...</b><span>waiting for the server</span></p>`}
        <h4 class="tr-sub">The journey</h4>
        <ol class="hops">
          ${journey.map(r => this.#hop(r, jmax)).join("")}
          ${at.ok && !at.ws && !at.replayed && at.state === "done" ? `<li class="hop hop-wait"><div class="hop-name"><b>Live update</b><span>Waiting for the WebSocket message that carries this bid.</span></div><div class="hop-val"><b class="tnum">...</b></div></li>` : ""}
          ${!at.queued ? `<li class="hop hop-off" title="Asynchronous bids are queued through Kafka, partitioned by auction so each lot keeps its order."><div class="hop-name"><b>Kafka bid queue</b><span>Not used for this bid. It took the direct path.</span></div><div class="hop-val"><b class="tnum">not used</b></div></li>` : ""}
        </ol>
        ${inside.length ? `<h4 class="tr-sub">Inside the transaction</h4><ol class="hops">${inside.map(r => this.#hop(r, imax)).join("")}</ol>` : ""}
        </section>
      ${this.#waterfall(at)}
      <section class="tr-sec" aria-label="Identifiers">
        <dl class="ids">
          ${this.#id("Request ID", at.requestId)}
          ${at.traceId ? this.#id("Trace ID", at.traceId, config.jaeger ? `${config.jaeger}/trace/${at.traceId}` : "") : `<div><dt>Trace ID</dt><dd class="muted">${be.kind === "sim" ? "No tracing in the demo engine" : "Not sent (tracing is off on this API)"}</dd></div>`}
          ${this.#id("Idempotency-Key", at.key)}
        </dl>
        ${at.outcome ? `<p class="tr-outcome tr-${esc(at.outcome.kind)}">${esc(at.outcome.message)}</p>` : ""}
      </section>`;
  }

  /** Real spans from Jaeger, when its address is configured; fetched once per trace. */
  #loadSpans(traceId) {
    if (!config.jaeger || !traceId || this.spans.has(traceId)) return;
    this.spans.set(traceId, { state: "loading" });
    fetch(`${config.jaeger}/api/traces/${traceId}`).then(r => (r.ok ? r.json() : Promise.reject(new Error("HTTP " + r.status)))).then(j => {
      const tr = j.data?.[0]; if (!tr) throw new Error("trace not found yet");
      const spans = tr.spans.map(s => ({ id: s.spanID, parent: s.references?.find(r => r.refType === "CHILD_OF")?.spanID, name: s.operationName, start: s.startTime / 1000, dur: s.duration / 1000 }));
      const t0 = Math.min(...spans.map(s => s.start)), byId = new Map(spans.map(s => [s.id, s]));
      const depth = s => { let d = 0, p = s.parent && byId.get(s.parent); while (p && d < 8) { d++; p = p.parent && byId.get(p.parent); } return d; };
      this.spans.set(traceId, { state: "ok", rows: spans.sort((a, b) => a.start - b.start).map(s => ({ d: Math.min(2, depth(s)), name: s.name.replace(/^db /, "SQL "), start: s.start - t0, dur: s.dur, fam: /^SQL|^db/.test(s.name) ? "" : "" })) });
      this.#schedule();
    }).catch(e => { this.spans.set(traceId, { state: "error", error: e.message }); this.#schedule(); });
  }

  #waterfall(at) {
    const real = at.traceId ? this.spans.get(at.traceId) : null;
    if (at.traceId) this.#loadSpans(at.traceId);
    const rows = real?.state === "ok" ? real.rows : waterfallRows(at);
    if (!rows.length) return "";
    const end = Math.max(...rows.map(r => r.start + r.dur), 1);
    const src = real?.state === "ok" ? "Spans of the real trace, from Jaeger." : this.be.kind === "sim" ? "Reconstructed from simulated timings." : "Reconstructed from the round trip, Server-Timing and event timestamps. Set a Jaeger address to draw the real spans.";
    return `
      <section class="tr-sec"><div class="tr-sechead"><h3>Trace waterfall</h3><span class="mono muted">${esc(dur(end))} end to end</span></div>
        <div class="wf" role="table" aria-label="Spans of this bid on one time axis">${rows.slice(0, 40).map(r => `
          <div class="wf-row depth-${r.d}" role="row"><span class="wf-name" role="cell">${esc(r.name)}</span>
            <span class="wf-track" role="cell"><i class="wf-span ${r.fam}" style="left:${(r.start / end * 100).toFixed(2)}%;width:${Math.max(0.6, r.dur / end * 100).toFixed(2)}%"></i></span>
            <span class="wf-ms" role="cell">${esc(dur(r.dur))}</span></div>`).join("")}</div>
        <p class="small muted">${esc(src)}${real?.state === "error" ? " Jaeger said: " + esc(real.error) + "." : ""}</p>
      </section>`;
  }

  #id(label, val, href) {
    if (!val) return `<div><dt>${esc(label)}</dt><dd class="muted">-</dd></div>`;
    return `<div><dt>${esc(label)}</dt><dd><code>${esc(short(val) + (val.length > 8 ? "..." : ""))}</code><button class="linkish" data-act="copy" data-val="${esc(val)}">Copy</button>${href ? `<a class="linkish" href="${esc(href)}" target="_blank" rel="noopener">Open trace</a>` : ""}</dd></div>`;
  }

  #attempts() {
    const list = this.s.attempts; if (list.length < 2) return "";
    return `
      <section class="tr-sec"><div class="tr-sechead"><h3>This session</h3><span class="mono muted">${list.length} attempts</span></div>
        <ul class="tr-list">${list.map(a => `
          <li><button data-act="pick" data-id="${esc(a.id)}" aria-pressed="${(this.sel || list[0].id) === a.id}">
            <span class="tnum">${usd(a.amount)}</span>
            <span class="tag ${a.state === "sent" ? "tag-warn" : a.ok ? (a.replayed ? "tag-warn" : "tag-ok") : "tag-bad"}">${a.state === "sent" ? "sending" : a.ok ? (a.replayed ? "replayed" : "placed") : esc(a.outcome?.kind || "refused")}</span>
            <span class="tnum muted">${a.rtt != null ? esc(dur(a.rtt)) : ""}</span>
            <code class="muted">${esc(short(a.key))}</code>
          </button></li>`).join("")}</ul>
      </section>`;
  }

  #events() {
    const me = this.s.me, list = this.s.log.slice(0, 12);
    return `
      <section class="tr-sec"><div class="tr-sechead"><h3>Live updates on this lot</h3><span class="mono muted">${this.be.feed.state === "open" ? "WebSocket open" : esc(this.be.feed.state)}</span></div>
        ${list.length ? `<ol class="evlog">${list.map(e => `
          <li><time class="mono">${esc(timeOnly(new Date(e.at).toISOString()))}</time><code>v${e.version}</code><span>${esc(e.cause)}</span>
            <span class="tnum">${e.amount != null ? usd(e.amount) : ""}</span><span class="muted">${e.bid ? esc(bidderLabel(e.bid.bidder_id, me)) : ""}</span>
            <span class="tnum muted" title="Time the event spent between the commit and the push, both from the API's clock">${e.timing ? esc(dur(Date.parse(e.timing.sent_at) - Date.parse(e.timing.placed_at))) : ""}</span></li>`).join("")}</ol>
          <p class="small muted">Each message carries the auction version. A message older than the one on screen is dropped, so the page never goes backwards.</p>`
          : `<p class="small muted">Waiting for the first update. Bids from any window appear here as they commit.</p>`}
      </section>`;
  }

  #stressHTML() {
    const st = this.stress, running = st.state === "running";
    const stat = (l, v) => `<div><dt>${l}</dt><dd class="tnum">${v}</dd></div>`;
    return `
      <section class="tr-sec tr-stress" id="stress">
        <div class="tr-sechead"><h3>Stress it</h3><span class="mono muted">${this.be.kind === "sim" ? "Simulated bidders" : "Real HTTP bidders"}</span></div>
        <p class="small muted">Many bidders read the same price and bid in the same instant. Exactly one wins each round; the rest are told the new minimum. Then the page checks that nothing went wrong.</p>
        <div class="tr-controls">
          <label class="field"><span class="field-label">Bidders</span><select id="stN" ${running ? "disabled" : ""}>${[50, 100, 200].map(n => `<option ${n === (st.bidders || 200) ? "selected" : ""}>${n}</option>`).join("")}</select></label>
          <label class="field"><span class="field-label">Rounds</span><select id="stR" ${running ? "disabled" : ""}>${[4, 8, 12].map(n => `<option ${n === (st.rounds || 8) ? "selected" : ""}>${n}</option>`).join("")}</select></label>
          <button class="btn ${running ? "busy" : ""}" data-act="stress" ${running ? "disabled" : ""}>${st.state === "done" ? "Run again" : "Start the storm"}</button>
        </div>
        ${st.state === "error" ? `<p class="form-error" role="alert">${esc(st.error)}</p>` : ""}
        ${st.state === "running" || st.state === "done" ? `
          <dl class="tr-stats">
            ${stat("Round", `${st.round ?? 0} / ${st.rounds}`)}${stat("Accepted", st.accepted ?? 0)}${stat("Told to retry", st.rejected ?? 0)}
            ${stat("Bids per second", st.perSecond ? Math.round(st.perSecond) : "-")}${stat("p50", st.p50 != null ? dur(st.p50) : "-")}${stat("p95", st.p95 != null ? dur(st.p95) : "-")}
          </dl>` : ""}
        ${st.state === "done" ? `
          <p class="tr-big"><b class="tnum">${usd(st.finalBid)}</b><span>final bid after ${st.bidCount} accepted bids in ${st.elapsed?.toFixed(1)} s</span></p>
          <ul class="checks">${(st.checks || []).map(c => `<li class="${c.ok ? "ok" : "bad"}"><b>${c.ok ? "Pass" : "Fail"}</b><span>${esc(c.name)}</span><em class="muted">${esc(c.detail || "")}</em></li>`).join("")}</ul>` : ""}
      </section>`;
  }

  #demoControls() {
    return `
      <section class="tr-sec"><div class="tr-sechead"><h3>Demo controls</h3></div>
        <p class="small muted">The demo engine can move a lot's close, so the last minutes can be seen without waiting.</p>
        <button class="btn btn-line btn-sm" data-act="ff">Jump to the last 90 seconds</button>
      </section>`;
  }
}
