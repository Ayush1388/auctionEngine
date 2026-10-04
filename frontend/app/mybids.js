// "My bids" has no endpoint of its own, and does not need one: the wallet ledger
// records every RESERVE (a bid that led), RELEASE (outbid) and SETTLE (won)
// against an auction. The set of auctions in my ledger is the set I bid on.
import { toLot } from "./model.js";

/** IDs of every auction the signed-in user has bid on, newest activity first. */
export async function participatedIds(be, maxPages = 3) {
  const ids = [];
  let before;
  for (let i = 0; i < maxPages; i++) {
    const page = await be.ledger({ limit: 200, before });
    for (const e of page.entries) if (e.auction_id && !ids.includes(e.auction_id)) ids.push(e.auction_id);
    if (page.next_before == null) break;
    before = page.next_before;
  }
  return ids;
}

/** Where I stand on a lot. */
export function standing(lot, meId) {
  const mine = lot.bidderId === meId;
  if (lot.status === "ACTIVE" || lot.status === "NOT_ACTIVE") return mine ? "leading" : "outbid";
  if (lot.status === "COMPLETED") return mine ? "won" : "lost";
  return "closed";
}

/** Every auction I bid on, with my own top bid and where I stand. */
export async function loadMyBids(be) {
  const me = be.session.id;
  if (!me) return [];
  const ids = (await participatedIds(be)).slice(0, 40);
  const rows = await Promise.all(ids.map(async id => {
    try {
      const [a, h] = await Promise.all([be.getAuction(id), be.bids(id, { limit: 100 })]);
      const lot = toLot(a), mine = h.bids.filter(b => b.user_id === me);
      return { lot, standing: standing(lot, me), myTop: mine.reduce((m, b) => Math.max(m, b.amount), 0), myCount: mine.length, lastAt: mine[0]?.created_at || lot.createdAt };
    } catch { return null; }
  }));
  return rows.filter(Boolean).sort((a, b) => new Date(b.lastAt) - new Date(a.lastAt));
}
