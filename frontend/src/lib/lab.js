// Helpers shared by the under-the-hood pages (trust, chaos, system, race, search lab).
import { toLot } from "../../app/model.js";

/** Open lots a person may bid on (not their own), as { id, raw, lot }. */
export async function openLots(be) {
  const me = be.session.id;
  const all = await be.allAuctions({ status: "ACTIVE" }, 3);
  return all.filter(a => a.owner_id !== me && Date.parse(a.ends_at) - Date.now() > 20_000).map(a => ({ id: a.id, raw: a, lot: toLot(a) }));
}

/** "1 bid" / "3 bids" */
export const plural = (n, one, many = one + "s") => `${n} ${n === 1 ? one : many}`;
