// What one box of the architecture map is, how it is doing, and what happens if it fails.
import { FAULTS } from "../../app/faults.js";
import { NODE_LABEL, nodeFacts } from "../lib/archmap.js";
import { fam, sum } from "../hooks/system.js";
import { dur } from "../../app/util.js";

const NODE_FAULT = { redis: "redis", kafka: "kafka", elasticsearch: "elasticsearch", bidding: "bidding", postgres: "postgres_slow" };
const STATE = { ok: "Answering", warn: "Degraded", bad: "Down", off: "Not configured" };

function stats(id, state, be) {
  const m = state.metrics, lb = state.lastBid, ls = state.lastSearch, rows = [];
  const add = (k, v) => { if (v != null) rows.push([k, v]); };
  if (id === "gateway") add("Liveness round trip", state.ms != null ? dur(state.ms) : "no answer");
  if (id === "bidding") { add("Last bid, whole call", lb?.timing?.bidding != null ? dur(lb.timing.bidding) : null); add("Last bid outcome", lb ? String(lb.status) : null); }
  if (id === "postgres" && lb?.timing?.lock != null) { const t = lb.timing; add("Last bid: lock", dur(t.lock)); add("Last bid: write and commit", dur((t.write || 0) + (t.commit || 0))); }
  if (id === "outbox") { add("Waiting", m ? String(sum(fam(m, "auction_outbox_pending"))) : null); add("Dead-lettered", m ? String(sum(fam(m, "auction_outbox_dead_lettered"))) : null); }
  if (id === "kafka") add("Consumer lag", m ? String(sum(fam(m, "auction_kafka_consumer_lag"))) : null);
  if (id === "websocket") { add("Connection", be.feed.state); add("Connections open", m ? String(sum(fam(m, "auction_ws_connections"))) : null); add("Events seen on this page", String(state.events)); }
  if (id === "elasticsearch" || id === "redis") add("Search answered by", ls ? ls.backend : null);
  if (id === "elasticsearch") add("Last search", ls ? dur(ls.ms) : null);
  return rows;
}

export default function NodePanel({ id, health, state, be }) {
  const n = id && nodeFacts[id] ? id : "bidding";
  const st = health[n] || "ok", f = FAULTS.find(x => x.name === NODE_FAULT[n]);
  return (
    <div className="node-panel" aria-live="polite">
      <h3>{NODE_LABEL[n]}</h3>
      <p>{nodeFacts[n]}</p>
      <dl>
        <div><dt>Health</dt><dd>{STATE[st] || st}</dd></div>
        {stats(n, state, be).map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
      </dl>
      {f ? <p className="small"><b>If it fails:</b> {f.degrades}</p>
        : n === "postgres" ? <p className="small"><b>If it fails:</b> Nothing can decide money, so readiness fails and the instance leaves the load balancer. It is the one dependency without a fallback, by design.</p> : null}
    </div>
  );
}
