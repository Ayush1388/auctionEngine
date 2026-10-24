// Architecture map: the live diagram, plus the Kafka lanes that show which partition each lot's
// bids travel through and how far behind the consumers are.
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import ArchMap from "../components/ArchMap.jsx";
import NodePanel from "../components/NodePanel.jsx";
import PageHead from "../components/PageHead.jsx";
import TourBar from "../components/TourBar.jsx";
import { useAuth } from "../hooks/backend.jsx";
import { fam, useSystem } from "../hooks/system.js";
import { useDocumentTitle } from "../hooks/use.js";
import { PARTITIONS, partitionOf } from "../../app/partition.js";
import { toLot } from "../../app/model.js";
import { usd, uuid } from "../../app/util.js";

export default function System() {
  useDocumentTitle("Marque | Architecture map");
  const sys = useSystem({ watch: 12 });
  const { be, state } = sys;
  const { signedIn, id: me, isAdmin } = useAuth();
  const [selected, setSelected] = useState("bidding");
  const [lots, setLots] = useState([]);
  const [note, setNote] = useState(signedIn ? "Bids anyone places on any open lot also make pulses." : "Sign in to place a bid. Bids from other windows still make pulses.");

  useEffect(() => {
    let on = true;
    be.allAuctions({ status: "ACTIVE" }, 1).then(list => { if (on) setLots(list.map(a => ({ ...toLot(a), part: partitionOf(a.id) })).slice(0, 12)); }).catch(() => { if (on) setLots([]); });
    return () => { on = false; };
  }, [be]);

  const { lag, known } = useMemo(() => {
    const lag = new Array(PARTITIONS).fill(0);
    let known = false;
    for (const s of fam(state.metrics, "auction_kafka_consumer_lag")?.samples || []) {
      const p = Number(s.labels?.partition);
      if (p >= 0 && p < PARTITIONS) { lag[p] += s.value ?? 0; known = true; }
    }
    return { lag, known };
  }, [state.metrics]);
  const mine = new Set(lots.map(l => l.part));

  const send = async () => {
    const a = (await be.allAuctions({ status: "ACTIVE" }, 1)).filter(x => x.owner_id !== me)[0];
    if (!a) return;
    const min = a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment;
    const amount = a.current_bidder_id === me ? a.current_bid + a.min_increment : min;
    const r = await be.placeBid(a.id, amount, { key: uuid() });
    sys.bid(r);
    setNote(r.ok ? `Placed ${usd(amount)} on ${toLot(a).title}. Select a box to see its numbers.` : `Refused: ${r.error?.message || r.status}.`);
  };

  return (
    <main className="page ops lab">
      <TourBar stage="events" />
      <PageHead title="Architecture map" action={<Link className="btn btn-line" to="/chaos">Chaos lab</Link>}>
        The services a bid passes through. A pulse is drawn when a real bid, search or live update happens, and each box says what this browser can honestly know about it.
      </PageHead>
      <div className="lab-controls" style={{ marginBottom: 16 }}>
        <button className="btn" disabled={!signedIn} onClick={send}>Place a bid and watch it travel</button>
        <span className="small muted">{note}</span>
      </div>
      <ArchMap ref={sys.map} health={sys.health} selected={selected} onSelect={setSelected} />
      <NodePanel id={selected} health={sys.health} state={state} be={be} />
      <section className="ops-sec" aria-labelledby="lnH" style={{ marginTop: "clamp(28px,4vw,52px)" }}>
        <h2 className="h2" id="lnH">Kafka lanes</h2>
        <p className="lead">Each lot is pinned to one of {PARTITIONS} partitions by a hash of its ID, and one consumer owns a partition. That is why bids on one lot are processed in order while different lots run in parallel. A lane lights up when its consumer is behind.</p>
        <div className="lanes">
          {lag.map((v, i) => (
            <div key={i} className={"lane" + (v > 0 ? " has-lag" : "") + (mine.has(i) ? " is-mine" : "")} title={`Partition ${i}: ${v} records waiting`}>
              <span>lane {i}</span><b className="tnum">{known ? v : "-"}</b>
              <span className="lane-q" aria-hidden="true">{Array.from({ length: Math.min(v, 12) }, (_, k) => <i key={k} />)}</span>
            </div>
          ))}
        </div>
        <p className="small muted">
          {known ? "Records waiting for a bid worker, per partition. Run Stress it on one lot: its lane fills and the others stay empty."
            : isAdmin ? "Waiting for the first metrics reading."
            : "Consumer lag is an operator metric. Sign in as the operator to see how far behind each lane is; the lot-to-lane mapping below is computed in your browser."}
        </p>
        <ul className="tbl-lite">{lots.map(l => <li key={l.id}><span>{l.title}</span><span className="mono">lane {l.part}</span></li>)}</ul>
      </section>
    </main>
  );
}
