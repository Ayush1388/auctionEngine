// Two auction formats beyond the English (ascending) auction the engine runs.
// Pure rules and a tiny double-entry ledger, so they can be tested without a page.
//
//   Dutch      the price starts high and falls on a schedule; the first buyer to
//              accept pays the price on the clock at that moment, and the sale ends.
//   Sealed     everyone submits one hidden bid; at the close the highest wins.
//              first price: the winner pays their bid.
//              second price (Vickrey): the winner pays the runner-up's bid, or the
//              reserve if that is higher, so bidding your true value is the best play.
//
// Neither is offered by the API. They exist here to show how the formats differ
// and that the same money rules (hold, release, settle, journals sum to zero) fit.

export function dutchPrice({ start, floor, step, intervalMs }, elapsedMs) {
  if (elapsedMs < 0) return start;
  return Math.max(floor, start - step * Math.floor(elapsedMs / intervalMs));
}

/** How long until the price reaches the floor. */
export function dutchDuration({ start, floor, step, intervalMs }) {
  return Math.ceil(Math.max(0, start - floor) / step) * intervalMs;
}

/**
 * @param {{id:string, amount:number, at:number}[]} bids
 * @param {{rule?: "first"|"second", reserve?: number}} o
 * @returns {{ sold: boolean, winner?: string, price?: number, ranked: object[], reason?: string }}
 */
export function settleSealed(bids, { rule = "second", reserve = 0 } = {}) {
  const ranked = [...bids].filter(b => Number.isInteger(b.amount) && b.amount > 0).sort((a, b) => b.amount - a.amount || a.at - b.at);
  if (!ranked.length) return { sold: false, ranked, reason: "no bids" };
  const top = ranked[0];
  if (top.amount < reserve) return { sold: false, ranked, reason: "the highest bid is below the reserve" };
  const price = rule === "first" ? top.amount : Math.max(ranked[1]?.amount ?? 0, reserve, 1);
  return { sold: true, winner: top.id, price: Math.min(price, top.amount), ranked };
}

/** Accounts per person: available and reserved. Every posting must sum to zero. */
export class FormatLedger {
  constructor() { this.acct = new Map(); this.journals = []; }
  open(id, funds = 0) { this.acct.set(id, { available: 0, reserved: 0 }); if (funds) this.post([[id, "available", funds], ["external", "external", -funds]]); }
  get(id) { return this.acct.get(id) || { available: 0, reserved: 0 }; }
  post(lines) {
    if (lines.reduce((s, l) => s + l[2], 0) !== 0) throw new Error("journal does not balance");
    for (const [id, account, amount] of lines) {
      if (id === "external") continue;
      const a = this.acct.get(id); if (!a) throw new Error("unknown account " + id);
      if (a[account] + amount < 0) throw new Error("insufficient funds");
    }
    for (const [id, account, amount] of lines) if (id !== "external") this.acct.get(id)[account] += amount;
    this.journals.push(lines);
  }
  hold(id, amount) { this.post([[id, "available", -amount], [id, "reserved", amount]]); }
  release(id, amount) { this.post([[id, "reserved", -amount], [id, "available", amount]]); }
  settle(buyer, seller, price) { this.post([[buyer, "reserved", -price], [seller, "available", price]]); }
  /** Money in the system, excluding the outside world. */
  total() { let t = 0; for (const a of this.acct.values()) t += a.available + a.reserved; return t; }
  balanced() { return this.journals.every(j => j.reduce((s, l) => s + l[2], 0) === 0); }
}
