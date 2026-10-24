// The pill at the bottom left of every page: which engine you are on, whether everything answers,
// the round trip, the demo-bots switch, and the way to the architecture map and chaos lab.
import { useEffect, useRef, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import { useAuth, useBackend } from "../hooks/backend.jsx";
import { useDismiss } from "../hooks/use.js";
import { useBots, usePulse } from "../hooks/shell.js";
import { toast } from "../lib/toast.js";

function Spark({ values }) {
  if (values.length < 2) return <svg className="pulse-spark" viewBox="0 0 240 36" preserveAspectRatio="none" aria-hidden="true" />;
  const hi = Math.max(...values, 1);
  const pts = values.map((v, i) => `${(i / (values.length - 1) * 240).toFixed(1)},${(34 - v / hi * 30).toFixed(1)}`).join(" ");
  return <svg className="pulse-spark" viewBox="0 0 240 36" preserveAspectRatio="none" aria-hidden="true"><polyline points={pts} fill="none" stroke="currentColor" strokeWidth="2" vectorEffect="non-scaling-stroke" /></svg>;
}

function BotsRow() {
  const be = useBackend();
  const { signedIn, isAdmin } = useAuth();
  const { state, setAmbient, refresh } = useBots();
  const [busy, setBusy] = useState(false);
  if (!be.bots) return null;
  const enabled = state?.enabled, on = !!state?.ambient?.on, canSwitch = enabled && (signedIn || be.kind === "sim") && !busy;
  const note = !state ? "Cannot reach the API."
    : !enabled ? "Not built into this API. Start it with DEMO_BOTS_ENABLED=true."
    : !(signedIn || be.kind === "sim") ? "Sign in to switch them on."
    : on ? `On: ${state.ambient.bids} bids placed so far. Stress it on a lot page uses them too.`
    : "Off. Switch on for rival bidders on every open lot. Stress it works either way.";
  const flip = async () => {
    setBusy(true);
    try { const s = await setAmbient(!on); toast(s.ambient.on ? "Demo bots on" : "Demo bots off"); }
    catch (e) { toast(e.message || "Could not change the demo bots"); }
    setBusy(false);
  };
  const reseed = async () => {
    setBusy(true);
    try { const r = await be.bots.seed(); toast(r.created ? `${r.created} fresh lots opened` : "Every car already has an open lot"); refresh(); }
    catch (e) { toast(e.message || "Could not refresh the demo cars"); }
    setBusy(false);
  };
  return (
    <div className="botsrow-wrap">
      <div className="botsrow">
        <div><b>Demo bots</b><span className="small">{note}</span></div>
        <button className="switch" role="switch" aria-checked={on} aria-label="Demo bots" disabled={!canSwitch} onClick={flip} />
      </div>
      {be.kind === "live" && enabled && isAdmin && <button className="linkish botsrow-seed" disabled={busy} onClick={reseed}>Refresh demo cars</button>}
    </div>
  );
}

export default function EngineBadge() {
  const be = useBackend();
  const live = be.kind === "live";
  const p = usePulse();
  const [open, setOpen] = useState(false);
  const [ws, setWs] = useState(be.feed.state);
  const ref = useRef(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(ref, open, close);
  useEffect(() => be.feed.on?.("state", s => setWs(s.state)), [be]);

  const ms = p?.ms;
  const list = p ? Object.entries({ api: ms != null ? "ok" : "failing", ...p.checks }) : [];
  const stateLabel = !p ? "checking" : { ok: "All systems answering", warn: `${p.failing.length} degraded`, bad: "Not answering" }[p.state];
  return (
    <div className="engine" ref={ref}>
      <button className={"engine-pill " + (live ? "is-live" : "is-demo")} id="enginePill" aria-expanded={open} aria-controls="enginePanel" data-state={p?.state} onClick={() => setOpen(o => !o)}>
        <i aria-hidden="true" /><span>{live ? "Live backend" : "Demo engine"}</span>
        <b className="tnum">{ms != null ? (ms < 10 ? ms.toFixed(1) : Math.round(ms)) + " ms" : p ? "down" : ""}</b>
      </button>
      {open && (
        <div className="pop pop-engine" id="enginePanel">
          <BotsRow />
          <div className="pulse">
            <div className="pulse-head"><b>System pulse</b><span className="mono">{stateLabel}</span></div>
            <Spark values={p?.history || []} />
            <ul className="pulse-list">{list.map(([k, v]) => <li key={k} className={v === "ok" ? "ok" : "bad"}><span>{k}</span><b className="mono">{v === "ok" ? "ok" : "failing"}</b></li>)}</ul>
            <div className="pop-actions"><Link className="btn btn-line btn-sm" to="/system" onClick={close}>Architecture map</Link><Link className="btn btn-line btn-sm" to="/chaos" onClick={close}>Chaos lab</Link></div>
          </div>
          {live ? (
            <>
              <h3>Connected to the real API</h3>
              <p>Every bid on this page goes to <b className="mono">{be.detail}</b>: PostgreSQL, the outbox and a WebSocket. Timings in the Behind the bid panel are measured.</p>
              <p className="engine-ws mono">WebSocket: {({ open: "open", connecting: "connecting", retrying: "reconnecting", closed: "closed", idle: "waiting for a lot" })[ws] || ws}</p>
              <div className="pop-actions"><a className="btn btn-line" href="?engine=sim">Use the demo engine</a></div>
            </>
          ) : (
            <>
              <h3>Demo engine</h3>
              <p>{be.fellBack ? <>The real API at <b className="mono">{be.fellBack.api.replace(/^https?:\/\//, "")}</b> did not answer, so the site is running on its built-in engine. </> : null}It follows the same rules as the Go backend and runs in this browser. Windows on this device share it, so you can bid from two at once. Timings are simulated and marked as such.</p>
              <div className="pop-actions"><a className="btn btn-line" href="?engine=live">Try the real API</a><button className="btn btn-line" onClick={() => { if (confirm("Reset the demo? Accounts, bids and balances go back to the start.")) be.reset(); }}>Reset demo</button></div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
