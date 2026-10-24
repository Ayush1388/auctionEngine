// "Behind the bid": the numbers of one bid's journey, worked out from what the browser can see.
//
// Nothing here is invented. Each row says where its number comes from:
//   measured            the browser's own clock
//   server timing       the Server-Timing header the API sends with the bid
//   event timestamps    timestamps on the WebSocket message (both from the API's clock)
//   simulated           the demo engine, which models these phases
// A step that cannot be seen from the browser is listed as such, not guessed.

export const PHASES = [
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
 * The timeline of one bid as a waterfall: each row is a span with a start offset and a duration,
 * drawn on one shared time axis. Offsets are reconstructed from what the browser can see (round
 * trip, Server-Timing, event timestamps); with a Jaeger address configured the real spans are
 * fetched and drawn instead.
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

/** A Jaeger trace as waterfall rows. */
export function jaegerRows(tr) {
  const spans = tr.spans.map(s => ({ id: s.spanID, parent: s.references?.find(r => r.refType === "CHILD_OF")?.spanID, name: s.operationName, start: s.startTime / 1000, dur: s.duration / 1000 }));
  const t0 = Math.min(...spans.map(s => s.start)), byId = new Map(spans.map(s => [s.id, s]));
  const depth = s => { let d = 0, p = s.parent && byId.get(s.parent); while (p && d < 8) { d++; p = p.parent && byId.get(p.parent); } return d; };
  return spans.sort((a, b) => a.start - b.start).map(s => ({ d: Math.min(2, depth(s)), name: s.name.replace(/^db /, "SQL "), start: s.start - t0, dur: s.dur, fam: "" }));
}
