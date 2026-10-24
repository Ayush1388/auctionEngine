// Proxy (maximum) bidding rule, a port of nextAutoBid in internal/bidding/proxy_rules.go.
//
// A bidder states the most they will pay. When another bid would take the
// lead, the engine answers with the smallest bid that keeps them ahead, up to
// that maximum. Pure data in, one decision out, so the same rule can be
// tested without a clock, a network or a database.
//
//   auction   { status, current_bid, current_bidder_id, starting_price, min_increment, owner_id }
//   proxies   [{ userId, max, since }]
//   funds     userId -> most they could hold on this auction (available, plus
//             their own hold here when they lead)
//   returns   { userId, amount } or null when the auction is at rest
//
// The higher maximum wins and pays one increment over the lower maximum (or
// its own maximum if that is less). A challenger with no opposing maximum just
// bids the minimum. A tie goes to whoever set their maximum first.
export function minimumBid(a) {
  return a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment;
}

export function nextAutoBid(a, proxies, funds) {
  if (a.status !== "ACTIVE") return null;
  const minNext = minimumBid(a), inc = Math.max(a.min_increment, 1);
  const cap = p => Math.min(p.max, funds(p.userId));
  let leaderCap = -1;
  const challengers = [];
  for (const p of proxies) {
    if (p.userId === a.owner_id) continue;
    if (a.current_bidder_id && p.userId === a.current_bidder_id) { leaderCap = cap(p); continue; }
    if (cap(p) >= minNext) challengers.push(p);
  }
  if (!challengers.length) return null;
  challengers.sort((x, y) => cap(y) - cap(x) || x.since - y.since);
  const c = challengers[0], cCap = cap(c), current = a.current_bid ?? 0;
  if (!a.current_bidder_id || leaderCap <= current) return { userId: c.userId, amount: minNext };
  if (cCap > leaderCap) return { userId: c.userId, amount: Math.min(cCap, leaderCap + inc) };
  return { userId: a.current_bidder_id, amount: Math.min(leaderCap, cCap + inc) };
}
