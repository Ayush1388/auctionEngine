// System status. Probes are public; metrics are the operator's (GET /v1/admin/metrics).
import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ChartTable, LineChart } from "../components/LineChart.jsx";
import PageHead from "../components/PageHead.jsx";
import TourBar from "../components/TourBar.jsx";
import { useAuth, useBackend } from "../hooks/backend.jsx";
import { fam, sum } from "../hooks/system.js";
import { useDocumentTitle, useInterval } from "../hooks/use.js";
import { quantile } from "../../app/stats.js";
import { dur } from "../../app/util.js";

const POLL = 2000, KEEP = 150;
const WINDOWS = [["60", "1 minute"], ["180", "3 minutes"]];
const STATE = { 0: ["Closed", "ok"], 1: ["Half open", "warn"], 2: ["Open", "bad"] };
const gauge = (s, name) => sum(fam(s, name));

/** Turn two snapshots into the numbers on screen. */
function derive(prev, cur, snaps) {
  const dt = (Date.parse(cur.time) - Date.parse(prev.time)) / 1000 || POLL / 1000;
  const acc = s => sum({ samples: (fam(s, "auction_bids_total")?.samples || []).filter(x => x.labels.result === "accepted") });
  const bps = Math.max(0, (acc(cur) - acc(prev)) / dt);
  // latency over the last few polls: the difference of two cumulative histograms is that window's histogram
  const h = s => fam(s, "auction_bid_duration_seconds")?.samples[0];
  const base = snaps[Math.max(0, snaps.length - 6)] || prev, a = h(cur), b = h(base);
  let p95 = null;
  if (a && b) { const total = (a.count ?? 0) - (b.count ?? 0); if (total > 0) p95 = quantile(a.buckets.map((x, i) => ({ le: x.le, count: x.count - (b.buckets[i]?.count ?? 0) })), total, 0.95) * 1000; }
  return { t: Date.parse(cur.time), bps, p95 };
}

const Tile = ({ label, value, note }) => <div className="tile"><span className="lab mono">{label}</span><b className="tnum">{value}</b>{note ? <span className="small muted">{note}</span> : null}</div>;

function Chart({ id, title, points, format, axisFormat, min, unit }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <LineChart title={title} points={points} format={format} axisFormat={axisFormat} min={min} />
      <details className="tview" open={open} onToggle={e => setOpen(e.currentTarget.open)}>
        <summary className="linkish">Show as a table</summary>
        <div>{open && <ChartTable points={points} format={format} unitLabel={unit} />}</div>
      </details>
    </>
  );
}

export default function Status() {
  useDocumentTitle("Marque | System status");
  const be = useBackend();
  const { signedIn, isAdmin } = useAuth();
  const [ready, setReady] = useState(null);
  const [live, setLive] = useState(null);
  const [err, setErr] = useState(null);
  const [windowSec, setWindowSec] = useState(60);
  const [paused, setPaused] = useState(false);
  const snaps = useRef([]);
  const series = useRef({ bps: [], p95: [], lag: [] });
  const [, bump] = useState(0);

  const poll = useCallback(async () => {
    if (paused || document.hidden) return;
    try { setReady(await be.readyz()); setErr(null); } catch (e) { setReady(null); setErr(e.message); }
    try { setLive(await be.livez()); } catch { setLive(null); }
    if (be.session.isAdmin) {
      try {
        const cur = await be.metrics();
        const s = snaps.current;
        if (s.length) {
          const d = derive(s.at(-1), cur, s), sr = series.current;
          sr.bps.push({ t: d.t, v: d.bps });
          sr.p95.push({ t: d.t, v: d.p95 ?? sr.p95.at(-1)?.v ?? 0 });
          sr.lag.push({ t: d.t, v: gauge(cur, "auction_kafka_consumer_lag") });
          for (const k of Object.keys(sr)) if (sr[k].length > KEEP) sr[k].shift();
        }
        s.push(cur); if (s.length > 12) s.shift();
      } catch (e) { setErr(e.message); }
    }
    bump(n => n + 1);
  }, [be, paused]);
  useEffect(() => { poll(); }, [poll, isAdmin]);
  useInterval(poll, POLL);

  const win = pts => pts.filter(p => p.t >= (pts.at(-1)?.t ?? 0) - windowSec * 1000);
  const checks = ready?.checks || {};
  const cur = snaps.current.at(-1);
  const bps = win(series.current.bps), p95 = win(series.current.p95), lag = win(series.current.lag);
  const last = a => a.at(-1)?.v ?? 0;
  const results = cur ? (fam(cur, "auction_bids_total")?.samples || []).map(x => [x.labels.result, x.value]).sort((a, b) => b[1] - a[1]) : [];
  const breakers = cur ? (fam(cur, "auction_circuit_breaker_state")?.samples || []).map(x => [x.labels.name, x.value]) : [];
  const dead = cur ? gauge(cur, "auction_outbox_dead_lettered") : 0;
  const msAxis = v => (v >= 1000 ? (v / 1000).toFixed(1) + " s" : Math.round(v) + " ms");

  return (
    <main className="page ops">
      <TourBar stage="observe" />
      <PageHead title="System status" action={<Link className="btn btn-line" to="/admin">Operator console</Link>}>
        What the engine is doing right now: health checks, bids per second, how long a bid takes, and what is waiting in the queues.
      </PageHead>

      <section className="ops-sec" aria-labelledby="hH"><h2 className="h2" id="hH">Health checks</h2>
        <ul className="probes">
          {[["Liveness", !!live, live ? `process is up, answered in ${dur(live.ms)}` : "no answer"],
            ["Readiness", ready?.status === "ok", ready ? `status: ${ready.status}` : err || "no answer"],
            ...Object.entries(checks).map(([k, v]) => [k.charAt(0).toUpperCase() + k.slice(1), v === "ok", v === "ok" ? "reachable" : "failing"])]
            .map(([name, ok, detail]) => <li key={name} className="probe"><span className={"state " + (ok ? "ok" : "bad")}>{ok ? "OK" : "Down"}</span><b>{name}</b><span className="muted small">{detail}</span></li>)}
        </ul>
        <p className="small muted">{be.kind === "sim" ? "Demo engine: these checks are simulated." : "Readiness drains before a shutdown, so a rolling deploy sheds no requests. Optional dependencies are reported but do not fail it."}</p>
      </section>

      {!isAdmin ? (
        <section className="ops-sec"><h2 className="h2">Live metrics</h2>
          <div className="empty"><h3>Metrics are for operators</h3><p>{signedIn ? "This account is not an operator." : "Sign in as the operator account to see bids per second, latency and queue depth."}</p>
            <Link className="btn" to="/account?next=/status">{signedIn ? "Switch account" : "Sign in"}</Link></div></section>
      ) : !cur ? <div className="skel" style={{ height: 240 }} /> : (
        <>
          <div className="ops-filters" role="group" aria-label="Chart window"><span className="mono lab">Window</span>
            {WINDOWS.map(([k, l]) => <button key={k} className="preset" aria-pressed={String(windowSec) === k} onClick={() => setWindowSec(+k)}>{l}</button>)}
            <button className="preset" aria-pressed={paused} onClick={() => setPaused(p => !p)}>{paused ? "Resume" : "Pause"}</button>
            <span className="small muted">Updates every 2 seconds. {snaps.current.length < 2 ? "Collecting the first readings." : ""}</span></div>

          <section className="ops-hero" aria-label="Throughput">
            <div><span className="mono lab">Bids accepted per second</span><b className="hero tnum">{last(bps).toFixed(1)}</b>
              <p className="small muted">Place a bid, or run Stress it on a lot, and watch this move.</p></div>
            <div className="chart-card"><Chart title="Bids per second" points={bps} format={v => v.toFixed(1)} axisFormat={v => (v >= 10 ? String(Math.round(v)) : v.toFixed(1))} min={1} unit="Bids per second" /></div>
          </section>

          <section className="ops-two">
            <div className="chart-card"><h2 className="h3">Bid latency, p95</h2><p className="small muted">95 percent of bids finish faster than this, including the wait for the auction's row lock.</p>
              <Chart title="p95 latency" points={p95} format={dur} axisFormat={msAxis} min={5} unit="p95 latency" /></div>
            <div className="chart-card"><h2 className="h3">Kafka consumer lag</h2><p className="small muted">Records waiting for a bid worker. It rises during a burst and returns to zero.</p>
              <Chart title="Records waiting" points={lag} format={v => String(Math.round(v))} axisFormat={v => (v >= 10 ? String(Math.round(v)) : v.toFixed(1))} min={4} unit="Records waiting" /></div>
          </section>

          <section className="ops-sec" aria-label="Queues and connections"><h2 className="h2">Queues and connections</h2>
            <div className="tiles">
              <Tile label="Outbox waiting" value={gauge(cur, "auction_outbox_pending")} note="events committed, not yet delivered" />
              <Tile label="Dead-lettered" value={dead} note={dead ? "needs the operator console" : "nothing parked"} />
              <Tile label="WebSocket connections" value={gauge(cur, "auction_ws_connections")} note="people watching live" />
              <Tile label="Database connections" value={`${gauge(cur, "auction_db_pool_acquired_connections")} / ${gauge(cur, "auction_db_pool_max_connections")}`} note="in use / pool size" />
              <Tile label="Latency now (p95)" value={last(p95) ? dur(last(p95)) : "-"} note="last few seconds" />
            </div>
          </section>

          <section className="ops-two">
            <div><h2 className="h2">Bid outcomes</h2>
              {results.length ? <table className="tbl"><thead><tr><th>Outcome</th><th className="r">Count</th></tr></thead><tbody>{results.map(([k, v]) => <tr key={k}><td>{k.replace(/_/g, " ")}</td><td className="r tnum">{v}</td></tr>)}</tbody></table> : <p className="muted">No bids since the process started.</p>}</div>
            <div><h2 className="h2">Circuit breakers</h2>
              <table className="tbl"><thead><tr><th>Dependency</th><th>State</th></tr></thead><tbody>{breakers.map(([k, v]) => <tr key={k}><td>{k}</td><td><span className={"state " + STATE[v][1]}>{STATE[v][0]}</span></td></tr>)}</tbody></table>
              <p className="small muted">When a dependency fails repeatedly its breaker opens: search falls back to PostgreSQL, rate limits fail open, and bids return a fast 503 instead of hanging.</p></div>
          </section>
        </>
      )}
      {err && isAdmin && <p className="form-error" role="alert">{err}</p>}
    </main>
  );
}
