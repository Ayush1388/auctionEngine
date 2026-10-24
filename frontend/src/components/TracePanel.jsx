// "Behind the bid": follow one bid from the click to everyone's screen. A drawer docked beside the
// bid box (never over it), so the live price stays in view while a bid is explained. It also
// holds Stress it: many bidders at one price in one instant, checked from the outside.
import { useEffect, useRef, useState } from "react";
import { bidderLabel } from "../../app/bidding.js";
import { config } from "../../app/config.js";
import { hopsFor, jaegerRows, waterfallRows } from "../lib/trace.js";
import { dur, short, timeOnly, usd } from "../../app/util.js";

function Hop({ r, max }) {
  const w = r.fam === "total" ? 100 : Math.max(1.2, r.ms / max * 100).toFixed(1);
  return (
    <li className={"hop hop-" + r.fam} title={r.desc}>
      <div className="hop-name"><b>{r.label}</b><span>{r.desc}</span></div>
      <div className="hop-val"><b className="tnum">{dur(r.ms)}</b><span className="mono">{r.src}</span></div>
      <div className="hop-bar" aria-hidden="true"><i style={{ width: `${w}%` }} /></div>
    </li>
  );
}

function Id({ label, val, href }) {
  const [copied, setCopied] = useState(false);
  if (!val) return <div><dt>{label}</dt><dd className="muted">-</dd></div>;
  return (
    <div><dt>{label}</dt><dd>
      <code>{short(val) + (val.length > 8 ? "..." : "")}</code>
      <button className="linkish" onClick={() => { navigator.clipboard?.writeText(val); setCopied(true); setTimeout(() => setCopied(false), 900); }}>{copied ? "Copied" : "Copy"}</button>
      {href ? <a className="linkish" href={href} target="_blank" rel="noopener">Open trace</a> : null}
    </dd></div>
  );
}

/** The spans of one bid on one time axis; real Jaeger spans when an address is configured. */
function Waterfall({ at, be }) {
  const [real, setReal] = useState(null);
  const trace = at.traceId;
  useEffect(() => {
    setReal(null);
    if (!config.jaeger || !trace) return undefined;
    let live = true;
    setReal({ state: "loading" });
    fetch(`${config.jaeger}/api/traces/${trace}`).then(r => (r.ok ? r.json() : Promise.reject(new Error("HTTP " + r.status)))).then(j => {
      const tr = j.data?.[0]; if (!tr) throw new Error("trace not found yet");
      if (live) setReal({ state: "ok", rows: jaegerRows(tr) });
    }).catch(e => { if (live) setReal({ state: "error", error: e.message }); });
    return () => { live = false; };
  }, [trace]);
  const rows = real?.state === "ok" ? real.rows : waterfallRows(at);
  if (!rows.length) return null;
  const end = Math.max(...rows.map(r => r.start + r.dur), 1);
  const src = real?.state === "ok" ? "Spans of the real trace, from Jaeger." : be.kind === "sim" ? "Reconstructed from simulated timings." : "Reconstructed from the round trip, Server-Timing and event timestamps. Set a Jaeger address to draw the real spans.";
  return (
    <section className="tr-sec"><div className="tr-sechead"><h3>Trace waterfall</h3><span className="mono muted">{dur(end)} end to end</span></div>
      <div className="wf" role="table" aria-label="Spans of this bid on one time axis">
        {rows.slice(0, 40).map((r, i) => (
          <div className={"wf-row depth-" + r.d} role="row" key={i}><span className="wf-name" role="cell">{r.name}</span>
            <span className="wf-track" role="cell"><i className={"wf-span " + r.fam} style={{ left: `${(r.start / end * 100).toFixed(2)}%`, width: `${Math.max(0.6, r.dur / end * 100).toFixed(2)}%` }} /></span>
            <span className="wf-ms" role="cell">{dur(r.dur)}</span></div>
        ))}
      </div>
      <p className="small muted">{src}{real?.state === "error" ? ` Jaeger said: ${real.error}.` : ""}</p>
    </section>
  );
}

function Attempt({ at, be }) {
  const { journey, inside } = hopsFor(at, be);
  const e2e = journey.find(r => r.id === "e2e");
  const jmax = Math.max(1, ...journey.map(r => r.ms)), imax = Math.max(0.001, ...inside.map(r => r.ms));
  const state = at.state === "sent" ? "In flight" : at.ok ? (at.replayed ? "Replayed, no new bid" : `${at.status} ${at.status === 201 ? "Created" : "OK"}`) : `${at.status || "No answer"} ${at.error?.message || ""}`;
  return (
    <>
      <section className="tr-sec" aria-labelledby="trH">
        <div className="tr-sechead"><h3 id="trH">Bid {usd(at.amount)}</h3><span className={"tag " + (at.state === "sent" ? "tag-warn" : at.ok ? "tag-ok" : "tag-bad")}>{state}</span></div>
        {e2e ? <p className="tr-big"><b className="tnum">{dur(e2e.ms)}</b><span>from your click to the live update on screen</span></p>
          : at.rtt != null ? <p className="tr-big"><b className="tnum">{dur(at.rtt)}</b><span>round trip{at.ok && !at.replayed ? "; waiting for the live update" : ""}</span></p>
          : <p className="tr-big"><b className="tnum">...</b><span>waiting for the server</span></p>}
        <h4 className="tr-sub">The journey</h4>
        <ol className="hops">
          {journey.map(r => <Hop key={r.id} r={r} max={jmax} />)}
          {at.ok && !at.ws && !at.replayed && at.state === "done" && <li className="hop hop-wait"><div className="hop-name"><b>Live update</b><span>Waiting for the WebSocket message that carries this bid.</span></div><div className="hop-val"><b className="tnum">...</b></div></li>}
          {!at.queued && <li className="hop hop-off" title="Asynchronous bids are queued through Kafka, partitioned by auction so each lot keeps its order."><div className="hop-name"><b>Kafka bid queue</b><span>Not used for this bid. It took the direct path.</span></div><div className="hop-val"><b className="tnum">not used</b></div></li>}
        </ol>
        {inside.length > 0 && <><h4 className="tr-sub">Inside the transaction</h4><ol className="hops">{inside.map(r => <Hop key={r.id} r={r} max={imax} />)}</ol></>}
      </section>
      <Waterfall at={at} be={be} />
      <section className="tr-sec" aria-label="Identifiers">
        <dl className="ids">
          <Id label="Request ID" val={at.requestId} />
          {at.traceId ? <Id label="Trace ID" val={at.traceId} href={config.jaeger ? `${config.jaeger}/trace/${at.traceId}` : ""} />
            : <div><dt>Trace ID</dt><dd className="muted">{be.kind === "sim" ? "No tracing in the demo engine" : "Not sent (tracing is off on this API)"}</dd></div>}
          <Id label="Idempotency-Key" val={at.key} />
        </dl>
        {at.outcome && <p className={"tr-outcome tr-" + at.outcome.kind}>{at.outcome.message}</p>}
      </section>
    </>
  );
}

function Stress({ s, be }) {
  const [st, setSt] = useState({ state: "idle" });
  const [bidders, setBidders] = useState(200);
  const [rounds, setRounds] = useState(8);
  const run = st.state === "running";

  const finish = async ev => {
    let checks = ev.invariants?.checks || [];
    setSt({ ...ev, state: "done", checks });
    // the page checks the result itself, from what the API returns, not only from what the bots say
    try {
      const h = await be.bids(s.id, { limit: 100 });
      const a = h.bids, ordered = a.every((b, i) => i === a.length - 1 || b.amount > a[i + 1].amount);
      checks = [{ name: "Bid history from the API is strictly increasing", ok: ordered, detail: `last ${a.length} bids read back from the server` }, ...checks.filter(c => !/strictly increasing/i.test(c.name))];
      if (be.session.isAdmin) {
        const r = await be.reconcile();
        checks = [...checks, { name: "Books reconcile (operator check)", ok: r.ok, detail: `${r.money_in_wallets} in wallets, ${r.reserved_in_wallets} held` }];
      }
      setSt(p => ({ ...p, checks }));
    } catch { /* the bots' own checks are still shown */ }
    s.reload();
  };
  const start = async () => {
    if (run) return;
    setSt({ state: "running", bidders, rounds, round: 0, accepted: 0, rejected: 0 });
    try {
      await be.stress.start({ auctionId: s.id, bidders, rounds }, ev => {
        if (ev.type === "progress") setSt(p => ({ ...p, ...ev, state: "running" }));
        else if (ev.type === "done") finish(ev);
        else if (ev.type === "error") setSt(p => ({ ...p, state: "error", error: ev.error }));
      });
    } catch (e) { setSt({ state: "error", error: e.message || "Could not start the run" }); }
  };
  const Stat = ({ l, v }) => <div><dt>{l}</dt><dd className="tnum">{v}</dd></div>;
  return (
    <section className="tr-sec tr-stress" id="stress">
      <div className="tr-sechead"><h3>Stress it</h3><span className="mono muted">{be.kind === "sim" ? "Simulated bidders" : "Real bidders, built into the API"}</span></div>
      <p className="small muted">Many bidders read the same price and bid in the same instant. Exactly one wins each round; the rest are told the new minimum. Then the page checks that nothing went wrong.</p>
      <div className="tr-controls">
        <label className="field"><span className="field-label">Bidders</span><select value={bidders} disabled={run} onChange={e => setBidders(+e.target.value)}>{[50, 100, 200].map(n => <option key={n}>{n}</option>)}</select></label>
        <label className="field"><span className="field-label">Rounds</span><select value={rounds} disabled={run} onChange={e => setRounds(+e.target.value)}>{[4, 8, 12].map(n => <option key={n}>{n}</option>)}</select></label>
        <button className={"btn" + (run ? " busy" : "")} disabled={run} onClick={start}>{st.state === "done" ? "Run again" : "Start the storm"}</button>
      </div>
      {st.state === "error" && <p className="form-error" role="alert">{st.error}</p>}
      {(st.state === "running" || st.state === "done") && (
        <dl className="tr-stats">
          <Stat l="Round" v={`${st.round ?? 0} / ${st.rounds}`} /><Stat l="Accepted" v={st.accepted ?? 0} /><Stat l="Told to retry" v={st.rejected ?? 0} />
          <Stat l="Bids per second" v={st.perSecond ? Math.round(st.perSecond) : "-"} /><Stat l="p50" v={st.p50 != null ? dur(st.p50) : "-"} /><Stat l="p95" v={st.p95 != null ? dur(st.p95) : "-"} />
        </dl>
      )}
      {st.state === "done" && (
        <>
          <p className="tr-big"><b className="tnum">{usd(st.finalBid)}</b><span>final bid after {st.bidCount} accepted bids in {st.elapsed?.toFixed(1)} s</span></p>
          <ul className="checks">{(st.checks || []).map(c => <li key={c.name} className={c.ok ? "ok" : "bad"}><b>{c.ok ? "Pass" : "Fail"}</b><span>{c.name}</span><em className="muted">{c.detail || ""}</em></li>)}</ul>
        </>
      )}
    </section>
  );
}

export default function TracePanel({ s, be, open, onClose }) {
  const [sel, setSel] = useState(null);
  const body = useRef(null);
  const close = useRef(null);
  useEffect(() => {
    if (!open) return undefined;
    close.current?.focus();
    const key = e => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", key);
    return () => document.removeEventListener("keydown", key);
  }, [open, onClose]);

  const sim = be.kind === "sim";
  const at = s.attempts.find(a => a.id === sel) || s.attempts[0];
  const me = s.me;
  return (
    <aside className={"trace" + (open ? " open" : "")} id="trace" aria-hidden={!open} role="dialog" aria-label="Behind the bid">
      <header className="trace-head">
        <div><h2>Behind the bid</h2><p className="mono">{sim ? "Demo engine. Timings are simulated." : "Live backend. Timings are measured."}</p></div>
        <button className="trace-x" ref={close} onClick={onClose} aria-label="Close">×</button>
      </header>
      {open && (
        <div className="trace-body" ref={body}>
          <p className="trace-intro">Place a bid, then watch it travel: the browser, the API, the bidding transaction in PostgreSQL, the outbox, and the WebSocket back to every screen. Each number says where it comes from.</p>
          {at ? <Attempt at={at} be={be} /> : <div className="trace-empty"><h3>No bid yet</h3><p>Your next bid appears here with its timeline.</p></div>}
          {s.attempts.length > 1 && (
            <section className="tr-sec"><div className="tr-sechead"><h3>This session</h3><span className="mono muted">{s.attempts.length} attempts</span></div>
              <ul className="tr-list">{s.attempts.map(a => (
                <li key={a.id}><button aria-pressed={(sel || s.attempts[0].id) === a.id} onClick={() => setSel(a.id)}>
                  <span className="tnum">{usd(a.amount)}</span>
                  <span className={"tag " + (a.state === "sent" ? "tag-warn" : a.ok ? (a.replayed ? "tag-warn" : "tag-ok") : "tag-bad")}>{a.state === "sent" ? "sending" : a.ok ? (a.replayed ? "replayed" : "placed") : (a.outcome?.kind || "refused")}</span>
                  <span className="tnum muted">{a.rtt != null ? dur(a.rtt) : ""}</span>
                  <code className="muted">{short(a.key)}</code>
                </button></li>))}</ul></section>
          )}
          <section className="tr-sec"><div className="tr-sechead"><h3>Live updates on this lot</h3><span className="mono muted">{be.feed.state === "open" ? "WebSocket open" : be.feed.state}</span></div>
            {s.log.length ? (
              <>
                <ol className="evlog">{s.log.slice(0, 12).map((e, i) => (
                  <li key={i}><time className="mono">{timeOnly(new Date(e.at).toISOString())}</time><code>v{e.version}</code><span>{e.cause}</span>
                    <span className="tnum">{e.amount != null ? usd(e.amount) : ""}</span><span className="muted">{e.bid ? bidderLabel(e.bid.bidder_id, me) : ""}</span>
                    <span className="tnum muted" title="Time the event spent between the commit and the push, both from the API's clock">{e.timing ? dur(Date.parse(e.timing.sent_at) - Date.parse(e.timing.placed_at)) : ""}</span></li>))}</ol>
                <p className="small muted">Each message carries the auction version. A message older than the one on screen is dropped, so the page never goes backwards.</p>
              </>
            ) : <p className="small muted">Waiting for the first update. Bids from any window appear here as they commit.</p>}
          </section>
          <Stress s={s} be={be} />
          {sim && (
            <section className="tr-sec"><div className="tr-sechead"><h3>Demo controls</h3></div>
              <p className="small muted">The demo engine can move a lot's close, so the last minutes can be seen without waiting.</p>
              <button className="btn btn-line btn-sm" onClick={() => be.fastForward(s.id, 90).catch(() => {})}>Jump to the last 90 seconds</button></section>
          )}
        </div>
      )}
    </aside>
  );
}
