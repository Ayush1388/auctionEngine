import test from "node:test";
import assert from "node:assert/strict";
import { dutchPrice, dutchDuration, settleSealed, FormatLedger } from "../app/formats.js";

const cfg = { start: 10_000, floor: 4_000, step: 500, intervalMs: 1000 };

test("a Dutch price falls on schedule and stops at the floor", () => {
  assert.equal(dutchPrice(cfg, 0), 10_000);
  assert.equal(dutchPrice(cfg, 999), 10_000);
  assert.equal(dutchPrice(cfg, 1000), 9_500);
  assert.equal(dutchPrice(cfg, 5_500), 7_500);
  assert.equal(dutchPrice(cfg, 999_999), 4_000);
  assert.equal(dutchDuration(cfg), 12_000);
});

test("sealed first price: the highest bid wins and pays its own bid; ties go to the earlier", () => {
  const r = settleSealed([{ id: "a", amount: 500, at: 3 }, { id: "b", amount: 900, at: 2 }, { id: "c", amount: 900, at: 1 }], { rule: "first" });
  assert.equal(r.winner, "c"); assert.equal(r.price, 900);
});

test("sealed second price: the winner pays the runner-up, never above their own bid, never below the reserve", () => {
  const bids = [{ id: "a", amount: 500, at: 1 }, { id: "b", amount: 900, at: 2 }];
  assert.deepEqual([settleSealed(bids).winner, settleSealed(bids).price], ["b", 500]);
  assert.equal(settleSealed(bids, { reserve: 700 }).price, 700);
  assert.equal(settleSealed(bids, { reserve: 950 }).sold, false);
  assert.equal(settleSealed([{ id: "a", amount: 800, at: 1 }]).price, 1);   // alone: pays the minimum, not their bid
});

test("the ledger holds, releases and settles with every journal summing to zero", () => {
  const l = new FormatLedger(); l.open("a", 1000); l.open("b", 1000); l.open("seller");
  l.hold("a", 600); l.hold("b", 900);
  assert.throws(() => l.hold("a", 500), /insufficient/);
  l.release("a", 600);
  l.settle("b", "seller", 500); l.release("b", 400);
  assert.deepEqual(l.get("seller"), { available: 500, reserved: 0 });
  assert.equal(l.get("b").available, 500);
  assert.equal(l.total(), 2000);
  assert.ok(l.balanced());
});
