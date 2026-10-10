// Run with: node --test frontend/tests
// The demo engine must follow the backend's rules, so these mirror the Go tests
// in internal/bidding and internal/wallet.
import test from "node:test";
import assert from "node:assert/strict";
import { World, SimError, SNIPE_WINDOW } from "../app/sim-engine.js";

const HOUR = 3600 * 1000;
function setup() {
  let t = Date.UTC(2027, 0, 1, 12, 0, 0);
  const w = new World({ now: () => t, rng: () => 0.5 });
  const advance = ms => { t += ms; w.tick(); };
  const seller = w.addUser({ email: "seller@x.test", funds: 0 });
  const a = w.addUser({ email: "a@x.test", funds: 100_000 });
  const b = w.addUser({ email: "b@x.test", funds: 100_000 });
  const mk = (over = {}) => {
    const auc = w.createAuction(seller.id, { item: { name: "Test car", type: "car", description: "" }, starting_price: 1000, min_increment: 100, starts_at: new Date(t + 5000).toISOString(), ends_at: new Date(t + HOUR).toISOString(), ...over });
    advance(6000);
    return auc;
  };
  return { w, advance, seller, a, b, mk, now: () => t };
}
const rejects = (fn, status, match) => assert.throws(fn, e => e instanceof SimError && e.status === status && (!match || match.test(e.message)));

test("minimum bid is the starting price, then current plus increment", () => {
  const { w, a, b, mk } = setup(); const au = mk();
  rejects(() => w.placeBid(a.id, au.id, 999), 422, /at least 1000/);
  assert.equal(w.placeBid(a.id, au.id, 1000).status, 201);
  rejects(() => w.placeBid(b.id, au.id, 1099), 422, /at least 1100/);
  assert.equal(w.placeBid(b.id, au.id, 1100).body.current_bid, 1100);
});

test("the leader's money is held, and released the moment someone outbids", () => {
  const { w, a, b, mk } = setup(); const au = mk();
  w.placeBid(a.id, au.id, 1000);
  assert.deepEqual(w.wallet(a.id), { available: 99_000, reserved: 1000, total: 100_000 });
  w.placeBid(b.id, au.id, 1100);
  assert.deepEqual(w.wallet(a.id), { available: 100_000, reserved: 0, total: 100_000 });
  assert.deepEqual(w.wallet(b.id), { available: 98_900, reserved: 1100, total: 100_000 });
});

test("raising your own bid reserves only the difference", () => {
  const { w, a, mk } = setup(); const au = mk();
  w.placeBid(a.id, au.id, 1000); w.placeBid(a.id, au.id, 1500);
  assert.deepEqual(w.wallet(a.id), { available: 98_500, reserved: 1500, total: 100_000 });
});

test("insufficient funds, own auction and ended auctions are refused", () => {
  const { w, a, seller, advance, mk } = setup(); const au = mk();
  rejects(() => w.placeBid(a.id, au.id, 200_000), 422, /insufficient funds/);
  rejects(() => w.placeBid(seller.id, au.id, 1000), 403, /own auction/);
  advance(2 * HOUR);
  rejects(() => w.placeBid(a.id, au.id, 1000), 409, /ended/);
});

test("the same Idempotency-Key returns the first bid and places nothing new", () => {
  const { w, a, mk } = setup(); const au = mk();
  const first = w.placeBid(a.id, au.id, 1000, "k1"), again = w.placeBid(a.id, au.id, 1000, "k1");
  assert.equal(first.status, 201); assert.equal(again.status, 200); assert.equal(again.replayed, true);
  assert.equal(again.body.bid.id, first.body.bid.id);
  assert.equal(w.getAuction(au.id).bid_count, 1);
  assert.equal(w.wallet(a.id).reserved, 1000);
  rejects(() => w.placeBid(a.id, au.id, 1100, "k1"), 422, /different bid/);
});

test("anti-sniping: a bid in the last two minutes moves the close to now + 2 minutes", () => {
  const { w, a, b, advance, mk, now } = setup(); const au = mk();
  advance(HOUR - 6000 - 30_000);                                // 30 s left
  const r = w.placeBid(a.id, au.id, 1000);
  assert.equal(r.body.extended, true);
  assert.equal(Date.parse(r.body.ends_at), now() + SNIPE_WINDOW);
  const early = w.placeBid(b.id, au.id, 1100);                  // 2 minutes left: not inside the window
  assert.equal(early.body.extended, false);
});

test("closing pays the seller from the winner's reserve, once", () => {
  const { w, a, b, seller, advance, mk } = setup(); const au = mk();
  w.placeBid(a.id, au.id, 1000); w.placeBid(b.id, au.id, 1100);
  advance(2 * HOUR);
  assert.equal(w.getAuction(au.id).status, "COMPLETED");
  assert.equal(w.wallet(seller.id).available, 1100);
  assert.deepEqual(w.wallet(b.id), { available: 98_900, reserved: 0, total: 98_900 });
  advance(HOUR);
  assert.equal(w.wallet(seller.id).available, 1100);
  assert.equal(w.reconcile().ok, true);
});

test("validation mirrors the API: field errors, future start, duration limits", () => {
  const { w, seller, now } = setup();
  const bad = () => w.createAuction(seller.id, { item: { name: "", type: "car" }, starting_price: -1, starts_at: new Date(now() - 1000).toISOString(), ends_at: new Date(now()).toISOString() });
  try { bad(); assert.fail(); } catch (e) { assert.equal(e.status, 400); assert.ok(e.body.fields["item.name"]); assert.ok(e.body.fields.starting_price); assert.equal(e.body.fields.starts_at, "must be in the future"); }
  rejects(() => w.createAuction(seller.id, { item: { name: "x", type: "car" }, starting_price: 1, starts_at: new Date(now() + 5000).toISOString(), ends_at: new Date(now() + 5000 + 30_000).toISOString() }), 400);
});

test("accounts: 15 character passwords, activation, login, refresh rotation and reuse detection", () => {
  const { w } = setup();
  rejects(() => w.register("n@x.test", "short"), 400);
  const { activationToken } = w.register("n@x.test", "a-long-enough-password");
  rejects(() => w.login("n@x.test", "a-long-enough-password"), 403, /not activated/);
  w.activate(activationToken);
  rejects(() => w.login("n@x.test", "wrong"), 401);
  const s = w.login("n@x.test", "a-long-enough-password");
  assert.equal(w.authenticate(s.access_token).email, "n@x.test");
  const s2 = w.refresh(s.refresh_token);
  assert.notEqual(s2.refresh_token, s.refresh_token);
  rejects(() => w.refresh(s.refresh_token), 401);              // reuse: theft signal
  rejects(() => w.refresh(s2.refresh_token), 401);             // ...so the whole session is revoked
});

test("deposits are idempotent and every journal balances", () => {
  const { w, a } = setup();
  w.deposit(a.id, 500, "d1"); w.deposit(a.id, 500, "d1");
  assert.equal(w.wallet(a.id).available, 100_500);
  rejects(() => w.deposit(a.id, 5, ""), 400);
  rejects(() => w.deposit(a.id, 0, "d2"), 422);
});

test("500 random bids from 12 bidders on 3 auctions keep every invariant", () => {
  let seed = 7; const rng = () => (seed = (seed * 1664525 + 1013904223) % 4294967296) / 4294967296;
  const { w, seller, advance, mk } = setup();
  const users = Array.from({ length: 12 }, (_, i) => w.addUser({ email: `u${i}@x.test`, funds: 5_000_000 }));
  const auctions = [mk(), mk(), mk()];
  let accepted = 0;
  for (let i = 0; i < 500; i++) {
    const au = auctions[Math.floor(rng() * 3)], u = users[Math.floor(rng() * users.length)];
    const min = w.getAuction(au.id).current_bid == null ? 1000 : w.getAuction(au.id).current_bid + 100;
    const amount = min + (rng() < 0.3 ? -100 : 0) + Math.floor(rng() * 3) * 100;     // some too low on purpose
    try { w.placeBid(u.id, au.id, amount, rng() < 0.2 ? "k" + (i % 30) : ""); accepted++; } catch (e) { assert.ok(e instanceof SimError); }
    if (i % 100 === 0) assert.equal(w.reconcile().ok, true);
  }
  assert.ok(accepted > 100);
  for (const au of auctions) assert.equal(w.invariants(au.id).ok, true, JSON.stringify(w.invariants(au.id)));
  advance(3 * HOUR);
  assert.equal(w.reconcile().ok, true);
  assert.ok(w.wallet(seller.id).available > 0);
});

test("search tolerates a typo and suggest matches word starts", () => {
  const { w, mk } = setup();
  mk({ item: { name: "1964 Jaguar E-Type", type: "car", description: "A coupe" } });
  assert.equal(w.search("jagaur").auctions.length, 1);
  assert.equal(w.suggest("jag")[0].title, "1964 Jaguar E-Type");
  assert.equal(w.search("porsche").auctions.length, 0);
});

/* ---------- maximum (proxy) bidding: mirrors internal/bidding/proxy_test.go ---------- */
test("a maximum answers a manual bid at once, holds only the visible bid, and the higher maximum wins", () => {
  const { w, a, b, mk } = setup(); const au = mk();
  let st = w.setProxy(a.id, au.id, 5000);
  assert.equal(st.leading, true); assert.equal(st.current_bid, 1000);
  const r = w.placeBid(b.id, au.id, 2000);
  assert.equal(r.body.countered, true); assert.equal(r.body.current_bid, 2100);
  assert.deepEqual(w.wallet(a.id), { available: 97_900, reserved: 2100, total: 100_000 });
  assert.equal(w.wallet(b.id).reserved, 0);
  st = w.setProxy(b.id, au.id, 9000);                 // higher maximum: wins one increment over 5000
  assert.equal(st.leading, true); assert.equal(st.current_bid, 5100);
  assert.deepEqual(w.wallet(a.id), { available: 100_000, reserved: 0, total: 100_000 });
  assert.ok(w.getAuction(au.id).bid_count >= 3);
  assert.equal(w.reconcile().ok, true);
  assert.ok(w.bidsOf(au.id).bids.some(x => x.auto));
});

test("a maximum is capped by the funds, never beyond them", () => {
  const { w, a, b, mk } = setup(); const au = mk();
  w.addUser({ email: "poor@x.test", funds: 1300 });
  const poor = [...w.users.values()].find(u => u.email === "poor@x.test");
  w.setProxy(poor.id, au.id, 50_000);                 // wants far more than they have
  w.placeBid(a.id, au.id, 1100);                      // a pushes; the engine can only go to 1300
  w.placeBid(b.id, au.id, 1400);
  assert.equal(w.wallet(poor.id).available + w.wallet(poor.id).reserved, 1300);
  assert.ok(w.wallet(poor.id).available >= 0);
  assert.equal(w.reconcile().ok, true);
});

test("maximum bid rules: below the minimum, own auction, ended, and removal", () => {
  const { w, a, seller, advance, mk } = setup(); const au = mk();
  rejects(() => w.setProxy(a.id, au.id, 500), 422, /at least 1000/);
  rejects(() => w.setProxy(seller.id, au.id, 5000), 403, /own auction/);
  w.setProxy(a.id, au.id, 3000);
  assert.equal(w.proxyOf(a.id, au.id).max_amount, 3000);
  w.cancelProxy(a.id, au.id);
  rejects(() => w.proxyOf(a.id, au.id), 404);
  advance(2 * HOUR);
  rejects(() => w.setProxy(a.id, au.id, 5000), 409, /ended/);
});

test("many maximums settle to the same result whatever order they arrive in", () => {
  const maxes = [2000, 3000, 4000, 4500, 3500];
  const results = new Set();
  for (let run = 0; run < 12; run++) {
    const { w, seller, mk } = setup(); const au = mk();
    const users = maxes.map((_, i) => w.addUser({ email: `u${i}@x.test`, funds: 100_000 }));
    const order = users.map((u, i) => [Math.sin(run * 7 + i), i]).sort((x, y) => x[0] - y[0]).map(x => x[1]);
    for (const i of order) { try { w.setProxy(users[i].id, au.id, maxes[i]); } catch { /* none expected */ } }
    const g = w.getAuction(au.id);
    results.add(`${users.findIndex(u => u.id === g.current_bidder_id)}:${g.current_bid}`);
    assert.equal(w.reconcile().ok, true);
    assert.ok(seller);
  }
  assert.deepEqual([...results], ["3:4100"]);        // 4500 wins, one increment over 4000
});
