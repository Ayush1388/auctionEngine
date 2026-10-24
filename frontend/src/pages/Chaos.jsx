// Chaos lab: break one dependency at a time, keep bidding, and watch what the system does. The
// switches drive /v1/admin/chaos on the real API (started with CHAOS_ENABLED=true) or the modelled
// faults of the demo engine.
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import ArchMap from "../components/ArchMap.jsx";
import NodePanel from "../components/NodePanel.jsx";
import OperatorGate from "../components/OperatorGate.jsx";
import PageHead from "../components/PageHead.jsx";
import TourBar from "../components/TourBar.jsx";
import { useAuth } from "../hooks/backend.jsx";
import { fam, sum, useSystem } from "../hooks/system.js";
import { useDocumentTitle, useInterval } from "../hooks/use.js";
import { openLots, plural } from "../lib/lab.js";
import { toast } from "../lib/toast.js";
import { FAULTS } from "../../app/faults.js";
import { usd, dur, timeOnly, uuid } from "../../app/util.js";

const SEARCH = "mustng";            // a typo on purpose: Elasticsearch forgives it, PostgreSQL does not
const newStats = () => ({ bids: { ok: 0, fast503: 0, slow503: 0, other: 0 }, bidMs: [], search: { elasticsearch: 0, postgres: 0 }, searchHits: { es: null, pg: null } });
const median = a => { const v = [...a].sort((x, y) => x - y); return v.length ? v[Math.floor(v.length / 2)] : null; };

function ChaosLab() {
  const sys = useSystem({ watch: 10 });
  const { be, state } = sys;
  const { signedIn } = useAuth();
  const sim = be.kind === "sim";
  const [chaos, setChaos] = useState(null);
  const [err, setErr] = useState(null);
  const [busy, setBusy] = useState(null);
  const [lots, setLots] = useState([]);
  const [lotId, setLotId] = useState("");
  const [feed, setFeed] = useState([]);
  const [auto, setAuto] = useState(false);
  const [stats, setStats] = useState(newStats);
  const [selected, setSelected] = useState("redis");

  useEffect(() => {
    be.chaos.get().then(c => { setChaos(c); setErr(null); }).catch(e => setErr(e.status === 404 ? "This API does not have the chaos endpoints. Rebuild it from this branch." : e.message));
  }, [be]);
  useEffect(() => { if (state.chaos) setChaos(state.chaos); }, [state.chaos]);
  useEffect(() => {
    openLots(be).then(l => { l.sort((a, b) => a.lot.minNext - b.lot.minNext); setLots(l); setLotId(l[0]?.id || ""); }).catch(() => {});
  }, [be]);

  const active = useMemo(() => new Set((chaos?.faults || []).filter(f => f.active).map(f => f.name)), [chaos]);
  const can = !!(chaos && (chaos.enabled || sim));
  const log = (kind, status, detail, ms, tone) => setFeed(f => [{ at: new Date().toISOString(), kind, status, detail, ms, tone, k: Math.random() }, ...f].slice(0, 40));

  const toggle = async (name, value) => {
    setBusy(name);
    try {
      setChaos(await be.chaos.set(name, value));
      toast(`${FAULTS.find(f => f.name === name).label} ${value ? "broken" : "restored"}`);
      sys.poll();
    } catch (e) { setErr(e.message); toast(e.message); }
    setBusy(null);
  };
  const restore = async () => {
    try { setChaos(await be.chaos.reset()); setStats(newStats()); toast("Everything restored"); } catch (e) { toast(e.message); }
    sys.poll();
  };

  const bidProbe = async () => {
    if (!signedIn || !lotId) return;
    try {
      const a = await be.getAuction(lotId);
      const min = a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment;
      const amount = a.current_bidder_id === be.session.id ? a.current_bid + a.min_increment : min;
      const r = await be.placeBid(lotId, amount, { key: uuid() });
      sys.bid(r);
      setStats(s => {
        const b = { ...s.bids };
        if (r.ok) b.ok++; else if (r.status === 503) (r.ms < 60 ? b.fast503++ : b.slow503++); else b.other++;
        return { ...s, bids: b, bidMs: [...s.bidMs, r.ms].slice(-30) };
      });
      log("Bid", r.status, r.ok ? `placed ${usd(amount)}` : (r.error?.message || "refused"), r.ms, r.ok ? "tag-ok" : r.status === 503 ? "tag-bad" : "tag-warn");
    } catch (e) { log("Bid", "ERR", e.message, 0, "tag-bad"); }
  };
  const searchProbe = async () => {
    const t0 = performance.now();
    try {
      const p = await be.search(SEARCH, { status: "ACTIVE", limit: 10 });
      const ms = performance.now() - t0, backend = p.backend === "demo" ? "elasticsearch" : p.backend;
      setStats(s => ({ ...s, search: { ...s.search, [backend]: (s.search[backend] || 0) + 1 }, searchHits: { ...s.searchHits, [backend === "postgres" ? "pg" : "es"]: p.auctions.length } }));
      sys.search(backend, ms);
      log("Search", 200, `${p.auctions.length} results from ${backend === "postgres" ? "PostgreSQL" : "Elasticsearch"}`, ms, backend === "postgres" ? "tag-warn" : "tag-ok");
    } catch (e) { log("Search", e.status || "ERR", e.message, performance.now() - t0, "tag-bad"); }
  };
  useInterval(bidProbe, auto ? 3000 : null);
  useInterval(() => { if (!document.hidden) searchProbe(); }, 3000);
  useEffect(() => { searchProbe(); }, []);   // eslint-disable-line react-hooks/exhaustive-deps

  const observed = useMemo(() => {
    const out = [], s = stats;
    const brState = name => fam(state.metrics, "auction_circuit_breaker_state")?.samples.find(y => y.labels?.name === name)?.value ?? null;
    if (!active.size) out.push(["ok", <><b>Everything is healthy.</b> Switch a dependency off and keep bidding. {s.bids.ok ? `So far ${plural(s.bids.ok, "bid")} placed, median ${dur(median(s.bidMs))}.` : ""}</>]);
    if (active.has("elasticsearch")) out.push([s.search.postgres ? "warn" : "ok", s.search.postgres
      ? <><b>Search fell back to PostgreSQL.</b> {plural(s.search.postgres, "search", "searches")} answered there. The typo in "{SEARCH}" now finds {s.searchHits.pg ?? 0} cars instead of {s.searchHits.es ?? "several"}: PostgreSQL full-text search has no typo tolerance. Breaker: {({ 0: "closed", 1: "half open", 2: "open" })[brState("elasticsearch")] ?? "unknown"}.</>
      : <><b>Elasticsearch is failing.</b> Waiting for the next search to show the fallback.</>]);
    if (active.has("bidding")) out.push([s.bids.fast503 ? "ok" : "warn", s.bids.fast503
      ? <><b>The breaker is open.</b> {plural(s.bids.fast503, "bid")} refused instantly with 503, after {plural(s.bids.slow503, "slow failure")}. Nobody waits for a timeout and nothing was charged.</>
      : <><b>The bidding service is down.</b> Place a bid: the first few fail slowly, then the breaker opens and the rest fail at once.</>]);
    if (active.has("postgres_slow")) out.push(["warn", <><b>PostgreSQL is slow.</b> Median bid time is {s.bidMs.length ? dur(median(s.bidMs)) : "waiting for a bid"}. Bids still commit one at a time and the books still balance; they just hold the row lock longer.</>]);
    if (active.has("redis")) out.push(["warn", <><b>Redis is down.</b> The cache is bypassed, trending comes from PostgreSQL and rate limits fail open. Bids and live updates on this instance are unaffected, because nothing that decides money reads from Redis.</>]);
    if (active.has("kafka")) {
      const pend = sum(fam(state.metrics, "auction_outbox_pending"));
      out.push(["warn", <><b>Kafka is unreachable.</b> Synchronous bids are unaffected. {state.metrics ? `${plural(pend, "event")} waiting in the outbox` : "Events wait in the outbox"}, in order, and flow when it returns.</>]);
    }
    return out;
  }, [active, stats, state.metrics]);

  return (
    <>
      {sim ? <p className="banner">Simulated. The demo engine models these faults on the same rules as the Go services: breakers open after five failures and recover after ten seconds. For the real thing, start the API with <code>CHAOS_ENABLED=true</code>.</p>
        : chaos && !chaos.enabled ? <p className="banner warn">Chaos is switched off on this API. Restart it with <code>CHAOS_ENABLED=true</code> and the switches below start working. Nothing is broken now.</p> : null}
      {err && <p className="form-error" role="alert">{err}</p>}
      <div className="chaos-grid">
        <section aria-labelledby="swH"><h2 className="h2" id="swH">Dependencies</h2>
          <ul className="faults">
            {FAULTS.map(f => (
              <li key={f.name} className={"fault" + (active.has(f.name) ? " is-on" : "")}>
                <h3>{f.label}</h3>
                <p>{f.short}. {active.has(f.name) ? f.degrades : f.does + " Switch it on to see."}</p>
                <button className="switch" role="switch" aria-checked={active.has(f.name)} aria-label={`Break ${f.label}`} disabled={!can || busy === f.name} onClick={() => toggle(f.name, !active.has(f.name))} />
              </li>
            ))}
          </ul>
          <div className="lab-controls" style={{ marginTop: 16 }}>
            <button className="btn btn-line btn-sm" onClick={restore}>Restore everything</button>
            <span className="small muted">{active.has("postgres_slow") ? `Each statement is delayed ${chaos?.slow_query_ms ?? 40} ms.` : ""}</span>
          </div>
        </section>
        <section aria-labelledby="mapH"><h2 className="visually" id="mapH">What a bid touches</h2>
          <ArchMap ref={sys.map} health={sys.health} selected={selected} onSelect={setSelected} />
          <NodePanel id={selected} health={sys.health} state={state} be={be} />
        </section>
      </div>

      <section className="ops-sec" aria-labelledby="prH">
        <h2 className="h2" id="prH">Keep bidding while it is broken</h2>
        <div className="lab-controls">
          <label className="field"><span className="field-label">Lot</span>
            <select value={lotId} onChange={e => setLotId(e.target.value)}>{lots.map(l => <option key={l.id} value={l.id}>{l.lot.title}</option>)}</select></label>
          <button className="btn" disabled={!signedIn || !lots.length} onClick={bidProbe}>Place a bid</button>
          <label className="check"><input type="checkbox" checked={auto} onChange={e => setAuto(e.target.checked)} /> Bid every 3 seconds</label>
        </div>
        <p className="small muted">Each bid is the minimum, so your holds stay small. A search for <b>{SEARCH}</b> runs every 3 seconds in the background.</p>
      </section>

      <div className="split-2">
        <section className="ops-sec" aria-labelledby="obH"><h2 className="h2" id="obH">What the system did</h2>
          <ul className="observed" aria-live="polite">{observed.map(([tone, node], i) => <li key={i} className={tone}>{node}</li>)}</ul></section>
        <section className="ops-sec" aria-labelledby="fdH"><h2 className="h2" id="fdH">Requests</h2>
          <ol className="feed">{feed.map(l => <li key={l.k}><time>{timeOnly(l.at)}</time><span className="kind">{l.kind}</span><span className={"tag " + l.tone}>{l.status}</span><span>{l.detail}</span><span className="r tnum">{dur(l.ms)}</span></li>)}</ol></section>
      </div>
    </>
  );
}

export default function Chaos() {
  useDocumentTitle("Marque | Chaos lab");
  return (
    <main className="page ops lab">
      <TourBar stage="resilience" />
      <PageHead title="Chaos lab" action={<Link className="btn btn-line" to="/system">Architecture map</Link>}>
        Break one dependency at a time and keep bidding. Bids still land, a breaker opens, search falls back to PostgreSQL, and the page shows what happened.
      </PageHead>
      <OperatorGate what="Breaking dependencies is an operator action." next="/chaos"><ChaosLab /></OperatorGate>
    </main>
  );
}
