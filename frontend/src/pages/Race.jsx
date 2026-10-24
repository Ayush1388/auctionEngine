// Race theater: several bids on one price, and the row lock that decides who goes first.
//  - "Race for real" fires the bids at the backend and draws each one's lock wait and work from
//    the Server-Timing it answered with (simulated timings in the demo engine).
//  - "Model" replays the same race under each locking strategy so the difference is visible.
//  - The numbers in the charts are the ones measured in docs/PERFORMANCE.md.
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import PageHead from "../components/PageHead.jsx";
import TourBar from "../components/TourBar.jsx";
import { useAuth, useBackend } from "../hooks/backend.jsx";
import { useDocumentTitle } from "../hooks/use.js";
import { openLots, plural } from "../lib/lab.js";
import { usd, dur, uuid } from "../../app/util.js";

// docs/PERFORMANCE.md: Run 2 (pessimistic, pool of 20) and Run 3 (BID_LOCKING=optimistic), 32 workers, 20 s
const MEASURED = {
  pessimistic: { label: "Pessimistic", p99: 322.2, accepted: 4572, benchBids: 255, benchAccepted: 178 },
  optimistic: { label: "Optimistic", p99: 159.5, accepted: 4509, benchBids: 682, benchAccepted: 178 },
};

// Work per bid is taken from a real measurement when there is one (write + commit), else a typical 3.4 ms.
function model(strategy, n, work = 3.4) {
  const out = [];
  if (strategy === "pessimistic") {
    // the row lock is a queue: each bid waits for every bid ahead of it, including the rejected ones
    let t = 0;
    for (let i = 0; i < n; i++) {
      const w = i === 0 ? work : 0.35;               // a refused bid only reads and decides
      out.push({ who: i === 0 ? "Winner" : `Bid ${i + 1}`, wait: t, work: w, ok: i === 0 });
      t += w;
    }
  } else {
    // nobody waits: everyone reads, decides, then races to write. One compare-and-swap wins; the rest retry once with backoff
    for (let i = 0; i < n; i++) {
      const read = 0.4;
      if (i === 0) out.push({ who: "Winner", wait: 0, work: read + work, ok: true });
      else out.push({ who: `Bid ${i + 1}`, wait: read + work * 0.6, work: 1 + (i % 3) + 0.4, ok: false });
    }
  }
  return out;
}

function Lanes({ rows, scale, label }) {
  return (
    <div className="race-lanes" role="img" aria-label={label}>
      {rows.map((r, i) => (
        <div className="race-lane" key={i}>
          <span className="who">{r.who}</span>
          <div className="race-track">
            {r.wait > 0.01 && <span className="race-seg wait" style={{ left: 0, width: `${Math.max(0.8, r.wait / scale * 100)}%` }} title={`waiting for the lock ${dur(r.wait)}`} />}
            <span className={"race-seg " + (r.ok ? "work" : "lost")} style={{ left: `${r.wait / scale * 100}%`, width: `${Math.max(1.2, r.work / scale * 100)}%` }}>{r.ok ? "wins" : ""}</span>
          </div>
          <span className="ms tnum small">{dur(r.wait + r.work)}</span>
        </div>
      ))}
    </div>
  );
}

function Bars({ rows }) {
  const max = Math.max(...rows.flatMap(r => [r.a, r.b]));
  return (
    <div className="bars">
      {rows.map(r => (
        <div className="bar-group" key={r.label}><span className="small">{r.label}</span>
          <div className="bar"><i style={{ width: `${r.a / max * 100}%` }} /><b className="tnum">{r.fmt(r.a)}</b><span className="small muted">pessimistic</span></div>
          <div className="bar alt"><i style={{ width: `${r.b / max * 100}%` }} /><b className="tnum">{r.fmt(r.b)}</b><span className="small muted">optimistic</span></div></div>
      ))}
    </div>
  );
}

export default function Race() {
  useDocumentTitle("Marque | Race theater");
  const be = useBackend();
  const { signedIn } = useAuth();
  const sim = be.kind === "sim";
  const [strategy, setStrategy] = useState("pessimistic");
  const [n, setN] = useState(8);
  const [lots, setLots] = useState([]);
  const [lotId, setLotId] = useState("");
  const [running, setRunning] = useState(false);
  const [real, setReal] = useState(null);
  const [err, setErr] = useState(null);

  useEffect(() => { openLots(be).then(l => { l.sort((a, b) => a.lot.minNext - b.lot.minNext); setLots(l); setLotId(l[0]?.id || ""); }).catch(() => {}); }, [be]);

  const go = async () => {
    const lot = lots.find(l => l.id === lotId);
    if (!lot || running || !signedIn) return;
    setRunning(true); setErr(null);
    try {
      const raw = await be.getAuction(lot.id);
      const amount = raw.current_bid == null ? Math.max(raw.starting_price, 1) : raw.current_bid + raw.min_increment;
      // distinct bidders where the demo can make them; the same account on the real API (the second copy of a price is refused)
      const actors = [];
      for (let i = 0; i < n; i++) actors.push(sim && be.trust && i > 0 ? await be.trust.attacker(amount * 3) : null);
      if (sim) await be.chaos.setRateLimits(false);
      const t0 = performance.now();
      const answers = await Promise.all(actors.map(a => (a ? a.placeBid(lot.id, amount, uuid()) : be.placeBid(lot.id, amount, { key: uuid() }))));
      const total = performance.now() - t0;
      if (sim) await be.chaos.setRateLimits(true);
      setReal({ amount, total, rows: answers.map((r, i) => ({ who: i === 0 ? "You" : `Rival ${i}`, ok: r.ok, status: r.status, ms: r.ms, tm: r.serverTiming })) });
    } catch (e) { setErr(e.message); try { if (sim) await be.chaos.setRateLimits(true); } catch { /* ignore */ } }
    setRunning(false);
  };

  const realRows = (real?.rows || []).map(x => {
    const lock = x.tm?.lock ?? 0, work = x.tm ? (x.tm.decide || 0) + (x.tm.write || 0) + (x.tm.commit || 0) : x.ms;
    return { who: x.who, wait: lock, work, ok: x.ok };
  });
  const winner = real?.rows.find(x => x.ok)?.tm;
  const work = winner ? winner.write + winner.commit : undefined;
  const mod = model(strategy, n, work);
  const scaleM = Math.max(...mod.map(r => r.wait + r.work), 1) * 1.05;
  const scaleR = Math.max(...realRows.map(r => r.wait + r.work), 1) * 1.05;
  const losers = realRows.filter(r => !r.ok);
  const m = MEASURED;

  return (
    <main className="page ops lab">
      <TourBar stage="decision" />
      <PageHead title="Race theater" action={<Link className="btn btn-line" to="/trust?part=money">The money</Link>}>
        Two bidders, one price, one row lock. Watch who waits, then compare pessimistic and optimistic locking on the numbers from PERFORMANCE.md.
      </PageHead>
      <div className="race">
        <section aria-labelledby="rrH">
          <h2 className="h2" id="rrH">Race for real</h2>
          <p className="lead">{sim ? "Several accounts bid the same price at the same instant." : "Several copies of one bid are sent at the same instant."} Exactly one is accepted. The rest wait for the lock, then learn the price moved.</p>
          <div className="lab-controls">
            <label className="field"><span className="field-label">Lot</span><select value={lotId} onChange={e => setLotId(e.target.value)}>{lots.map(l => <option key={l.id} value={l.id}>{l.lot.title}</option>)}</select></label>
            <label className="field"><span className="field-label">Bidders</span><select value={n} onChange={e => setN(+e.target.value)}>{[2, 4, 8, 16].map(v => <option key={v}>{v}</option>)}</select></label>
            <button className={"btn" + (running ? " busy" : "")} disabled={running || !signedIn || !lots.length} onClick={go}>Start the race</button>
          </div>
          {!signedIn && <p className="form-error">Sign in to race. <Link className="linkish" to="/account?next=/race">Sign in</Link></p>}
          {err && <p className="form-error" role="alert">{err}</p>}
          {real ? (
            <>
              <Lanes rows={realRows} scale={scaleR} label="Measured race" />
              <p className="small muted"><span className="tag tag-warn">waiting for the lock</span> <span className="tag tag-ok">working</span> The first bid at {usd(real.amount)} won. {plural(losers.length, "bid")} waited a median {dur(losers.length ? losers.map(l => l.wait).sort((a, b) => a - b)[Math.floor(losers.length / 2)] : 0)} behind it. The whole burst took {dur(real.total)}. {sim ? "Timings are simulated." : "Timings come from the Server-Timing header."}</p>
            </>
          ) : <p className="small muted">Press start. Each bar is one bid: the amber part is time spent waiting for the auction's row lock, the red part is its own transaction.</p>}
        </section>

        <section aria-labelledby="mdH">
          <h2 className="h2" id="mdH">Same race, two strategies</h2>
          <div className="presets" role="group" aria-label="Locking strategy">
            {Object.entries(m).map(([k, v]) => <button key={k} className="preset" aria-pressed={strategy === k} onClick={() => setStrategy(k)}>{v.label}</button>)}
          </div>
          <Lanes rows={mod} scale={scaleM} label={`Model of ${n} bids under ${strategy} locking`} />
          <p className="small muted">
            {strategy === "pessimistic"
              ? "Pessimistic: SELECT ... FOR UPDATE makes bids queue on the auction row. Each waits for everyone ahead, then sees the price the winner set. Predictable, but a hot lot builds a queue."
              : "Optimistic: nobody waits. Everyone reads and decides, then writes with a version check. One wins; the rest retry with backoff, find the price moved, and are refused. No queue, but wasted work."}
            {" "}This is a model, drawn to scale from a typical {dur(work ?? 3.4)} write.
          </p>
        </section>
      </div>

      <section className="ops-sec" aria-labelledby="pfH" style={{ marginTop: "clamp(28px,4vw,52px)" }}>
        <h2 className="h2" id="pfH">What it cost, measured</h2>
        <p className="lead">From docs/PERFORMANCE.md: 32 closed-loop workers for 20 seconds, half the bids aimed at one hot lot, on two shared vCPUs.</p>
        <Bars rows={[
          { label: "Bid latency, p99 (ms)", a: m.pessimistic.p99, b: m.optimistic.p99, fmt: v => v.toFixed(0) + " ms" },
          { label: "Bids accepted in 20 s", a: m.pessimistic.accepted, b: m.optimistic.accepted, fmt: v => v.toLocaleString() },
          { label: "Bid attempts per second, hot lot (Go benchmark)", a: m.pessimistic.benchBids, b: m.optimistic.benchBids, fmt: v => v + " /s" },
          { label: "Accepted bids per second, hot lot (Go benchmark)", a: m.pessimistic.benchAccepted, b: m.optimistic.benchAccepted, fmt: v => "about " + v + " /s" },
        ]} />
        <p className="small muted">The finding: optimistic locking halves the tail latency, but accepted bids do not move. On one hot lot the auction itself, one price changing at a time, sets how many bids can be accepted. The strategy only decides whether the excess waits or is refused quickly.</p>
      </section>
    </main>
  );
}
