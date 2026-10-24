// The backend tour: where a walkthrough of the system starts. One diagram, eight stages in the
// order a bid meets them, each with what happens, what it is built with, why, and a live demo
// page that runs against the real backend or its in-browser twin.
import { useState } from "react";
import { Link } from "react-router-dom";
import ArchMap from "../components/ArchMap.jsx";
import { useAuth } from "../hooks/backend.jsx";
import { useSystem } from "../hooks/system.js";
import { useDocumentTitle } from "../hooks/use.js";
import { STAGES } from "../../app/tour.js";
import { usd, uuid } from "../../app/util.js";

export default function Tour() {
  useDocumentTitle("Marque | How the backend works");
  const sys = useSystem({ watch: 12 });
  const { be, state } = sys;
  const { signedIn, id: me } = useAuth();
  const sim = be.kind === "sim";
  const [lit, setLit] = useState([]);
  const [selected, setSelected] = useState("bidding");
  const [note, setNote] = useState(signedIn ? "" : "Sign in to place a bid. Bids from anyone make pulses.");

  const checks = state.ready?.checks || {};
  const items = [["API", state.ms != null], ...Object.entries(checks).map(([k, v]) => [k, v === "ok"])];

  const send = async () => {
    const a = (await be.allAuctions({ status: "ACTIVE" }, 1)).find(x => x.owner_id !== me);
    if (!a) { setNote("There is no open lot to bid on."); return; }
    const min = a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment;
    const amount = a.current_bidder_id === me ? a.current_bid + a.min_increment : min;
    const r = await be.placeBid(a.id, amount, { key: uuid() });
    sys.bid(r);
    setNote(r.ok ? `Placed ${usd(amount)}. Watch it cross the diagram.` : `Refused: ${r.error?.message || r.status}.`);
  };

  return (
    <main className="page ops lab tour">
      <header className="tour-hero">
        <h1>How the backend works</h1>
        <p>An auction engine in Go. Follow one bid from your click to every screen in eight stages, then break things on purpose. Each stage has a live demo and says what it is built with and why.</p>
        <div className="tour-actions">
          <Link className="btn" to={STAGES[0].page}>Start at stage 1</Link>
          <button className="btn btn-line" disabled={!signedIn} onClick={send}>Send a bid through the diagram</button>
          <span className="small muted">{note}</span>
        </div>
        <div className="tour-status">
          <span className={"tag " + (sim ? "tag-warn" : "tag-ok")}>{sim ? "Demo engine, simulated" : "Real backend"}</span>
          {items.map(([k, ok]) => <span key={k} className={"tag " + (ok ? "tag-ok" : "tag-bad")}>{k} {ok ? "ok" : "down"}</span>)}
        </div>
      </header>

      <section aria-labelledby="mapH">
        <h2 className="visually" id="mapH">The system</h2>
        <ArchMap ref={sys.map} health={sys.health} selected={selected} onSelect={setSelected} lit={lit} />
        <p className="small muted">Select a stage below to light the parts it uses. A pulse is drawn for every real bid and live update.</p>
      </section>

      <ol className="tour-stages">
        {STAGES.map(s => (
          <li key={s.id} className="tstage" tabIndex={-1} onPointerEnter={() => setLit(s.nodes)} onFocus={() => setLit(s.nodes)} onPointerLeave={() => setLit([])}>
            <span className="tstage-n tnum" aria-hidden="true">{s.n}</span>
            <div className="tstage-body">
              <h2>{s.title}</h2>
              <div className="tstage-cols">
                <div className="stack">
                  <p className="tstage-what">{s.what}</p>
                  <p className="tstage-why"><b>Why this way.</b> {s.why} <span className="mono muted">{s.adr}</span></p>
                </div>
                <div className="stack">
                  <ul className="chips" aria-label="Built with">{s.tech.map(t => <li key={t}>{t}</li>)}</ul>
                  <p className="tstage-demo"><b>In the demo.</b> {s.demo}</p>
                </div>
              </div>
              <div className="tstage-actions"><Link className="btn btn-line btn-sm" to={s.page}>Open stage {s.n}</Link></div>
            </div>
          </li>
        ))}
      </ol>

      <p className="small muted" style={{ marginTop: 28 }}>
        {sim ? "You are on the in-browser demo engine: it follows the same rules as the Go services, and every timing and fault here is simulated and labelled. Run the real API to measure."
          : "You are connected to the real API: timings are measured, and the faults are injected into the running services."}
      </p>
    </main>
  );
}
