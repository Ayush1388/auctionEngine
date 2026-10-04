// Finding lots on the backend.
//
// The site's links use short catalogue keys (lot.html?id=corvette). On the
// backend a lot is an auction with a UUID, and the key travels in its metadata
// block. These helpers connect the two.
import { toLot, loadCatalog } from "./model.js";
import { tab } from "./util.js";

export const isUuid = s => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s || "");

/** Every auction that carries a catalogue key, as lots: Map(key -> lot). The newest open one wins. */
export async function liveLots(be) {
  const map = new Map();
  for (const status of ["ACTIVE", "NOT_ACTIVE", "COMPLETED"]) {
    let list = [];
    try { list = await be.allAuctions({ status }, 3); } catch { return map; }
    for (const a of list) { const l = toLot(a); if (l.key && !map.has(l.key)) map.set(l.key, l); }
  }
  return map;
}

/** A page parameter (a catalogue key or an auction ID) -> an auction ID, or null. */
export async function resolveId(be, param) {
  if (isUuid(param)) return param;
  const memo = tab.json("marque-keys:" + be.kind + ":" + be.base, {});
  if (memo[param]) return memo[param];
  const lots = await liveLots(be), l = lots.get(param);
  if (l) { memo[param] = l.id; tab.setJson("marque-keys:" + be.kind + ":" + be.base, memo); }
  return l ? l.id : null;
}

/** The catalogue order of keys, for previous and next links. */
export async function catalogOrder() { return (await loadCatalog()).lots.map(l => l.key); }
