// The demo engine as a backend: sim-engine.js (the rules) plus everything a
// server has that a data structure does not.
//
//  - ONE auction house per browser. Every window on this device talks to the
//    same engine: the window that holds a Web Lock is the "host" and runs it;
//    the others send it requests over a BroadcastChannel and receive its live
//    events. Close the host window and another one takes over, restoring the
//    saved state. This is also how ordering works: all bids go through one
//    place, one at a time, like bids through one Kafka partition.
//  - simulated network and outbox delay, so loading states and the
//    "behind the bid" timings look like what a real server produces. Every
//    number the demo shows is labelled as simulated.
//  - rival bidders and the auction lifecycle (start, close, pay out) on timers
//  - the stress runner
import { World, SimError } from "./sim-engine.js";
import { ApiError } from "./api.js";
import { Session } from "./session.js";
import { loadCatalog, encodeDescription, metaOf, proseOf } from "./model.js";
import { Emitter, uuid, sleep, local } from "./util.js";
import { DEMO_ACCOUNTS, DEMO_PASSWORD } from "./demo.js";
import { FAULTS } from "./faults.js";
import { partitionOf, PARTITIONS } from "./partition.js";

const DEFAULT_FAULTS = { redis: false, kafka: false, elasticsearch: false, bidding: false, postgres_slow: false, slow_ms: 40, rate_limits: true };
const mkBreaker = () => ({ state: 0, fails: 0, openedAt: 0 });          // 0 closed, 1 half open, 2 open
const BREAKER_THRESHOLD = 5, BREAKER_COOLDOWN = 10_000;
const BUCKET = { rate: 5, burst: 10 };                                  // the API's per-user bid rule

const SAVE_KEY = "marque-sim-world-v2";
const LOCK = "marque-sim-host";
const CHANNEL = "marque-sim";
const PASSWORD = DEMO_PASSWORD;
const jitter = (lo, hi) => lo + Math.random() * (hi - lo);
const hex = n => Array.from({ length: n }, () => (Math.random() * 16 | 0).toString(16)).join("");
const D = 100;                       // dollars to cents


/* ================================================================== host */
class Host {
  constructor(node) {
    this.node = node; this.world = null; this.stress = new Map(); this.pendingEvents = 0; this.connections = new Map(); this.timers = [];
    this.br = { bidding: mkBreaker(), elasticsearch: mkBreaker() };
    this.rivalsOn = true; this.rivalStats = { bids: 0, refused: 0, since: Date.now(), lastAt: null, lastLot: "" };
    this.kafkaBacklog = 0; this.backlogParts = new Array(PARTITIONS).fill(0); this.stressPart = 0; this.buckets = new Map();
  }

  async start() {
    const saved = local.json(SAVE_KEY);
    try { if (saved && saved.v === 1) this.world = World.fromJSON(saved); } catch { this.world = null; }
    if (!this.world) { this.world = new World(); await this.#seed(); this.save(); }
    this.rivals = [...this.world.users.values()].filter(u => u.bot && u.email.startsWith("bidder-"));
    this.world.onEvent(ev => this.#publish(ev));
    this.timers.push(setInterval(() => this.world.tick(), 1000));
    this.timers.push(setInterval(() => this.#rival(), 7000));
    this.timers.push(setInterval(() => this.save(), 4000));
    this.timers.push(setInterval(() => { if (!this.node.faults.kafka && this.kafkaBacklog > 0) { this.backlogParts = this.backlogParts.map(v => Math.max(0, v - 8)); this.kafkaBacklog = this.backlogParts.reduce((a, b) => a + b, 0); } }, 500));
    addEventListener("pagehide", () => this.save());
  }
  stop() { this.timers.forEach(clearInterval); this.timers = []; }

  save() { try { local.set(SAVE_KEY, JSON.stringify(this.world)); } catch { /* quota: the demo keeps running unsaved */ } }
  reset() { local.del(SAVE_KEY); this.stop(); this.world = null; }

  async #seed() {
    const w = this.world, cat = await loadCatalog(), now = Date.now();
    const house = w.addUser({ email: "house@marque.test", password: uuid(), name: "Marque House" });
    this.rivals = [184, 291, 407, 52, 318, 96].map(n => w.addUser({ email: `bidder-${n}@bots.marque.test`, password: uuid(), name: `Bidder #${n}`, funds: 20_000_000 * D, bot: true }));
    DEMO_ACCOUNTS.forEach(a => w.addUser({ email: a.email, password: PASSWORD, name: a.name, role: a.role, funds: 2_000_000 * D }));
    for (const lot of cat.lots) {
      const start = (lot.bid - lot.bids * lot.step) * D, step = lot.step * D;
      const a = w.createAuction(house.id, {
        item: { name: lot.title, type: "car", description: encodeDescription(proseOf(lot), metaOf(lot)) },
        starting_price: start, min_increment: step,
        starts_at: new Date(now - 6 * 3600e3).toISOString(), ends_at: new Date(now + lot.endsIn * 1000).toISOString(),
      }, { seed: true });
      // real history through the real rules, back-dated across the last five hours
      for (let k = 0; k < lot.bids; k++) {
        const who = this.rivals[k % this.rivals.length], at = now - 5 * 3600e3 + (k / lot.bids) * 4.9 * 3600e3;
        w.placeBid(who.id, a.id, start + (k + 1) * step, "", { at });
      }
    }
    w.counters = { bids: {}, durations: [], settled: 0, events: 0 };        // history is not traffic
    this.#deadLetters();
    w.tick();
  }

  #deadLetters() {
    const t = Date.now();
    this.world.deadLetters.push(
      { id: uuid(), event_type: "email.activation", attempts: 8, last_error: "dial tcp 10.0.3.14:1025: connect: connection refused", failed_at: new Date(t - 52 * 60e3).toISOString() },
      { id: uuid(), event_type: "auction.indexed", attempts: 8, last_error: "elasticsearch: 503 no living connections", failed_at: new Date(t - 19 * 60e3).toISOString() },
    );
  }

  /** An update leaves the "outbox" a few ms after the commit, then fans out to every window. */
  #publish(ev) {
    this.pendingEvents++;
    setTimeout(() => {
      this.pendingEvents--;
      if (ev.timing) ev.timing.sent_at = new Date().toISOString();
      this.node.bc.postMessage({ t: "event", ev });
      this.node.deliver(ev);
    }, jitter(2, 6));
  }

  /** A rival bids on a random open lot now and then, so a page left open stays alive. */
  #rival() {
    if (!this.rivalsOn) return;
    const w = this.world, live = [...w.auctions.values()].filter(a => a.status === "ACTIVE" && Date.parse(a.ends_at) - Date.now() > 20000 && !a.item.name.startsWith("Stress"));
    if (!live.length) return;
    const a = live[Math.floor(Math.random() * live.length)];
    const who = this.rivals.filter(r => r.id !== a.current_bidder_id)[Math.floor(Math.random() * (this.rivals.length - 1))];
    try {
      w.placeBid(who.id, a.id, w.minimum(a), "", { queue: 0 });
      this.rivalStats.bids++; this.rivalStats.lastAt = Date.now(); this.rivalStats.lastLot = a.item.name;
    } catch { this.rivalStats.refused++; /* a rival that can't afford it just skips */ }
  }

  /* ---------- one RPC, run on the host ---------- */
  dispatch(method, args, token) {
    const w = this.world;
    const me = () => w.authenticate(token);
    switch (method) {
      case "register": return w.register(...args);
      case "activate": return w.activate(args[0]);
      case "resend": return { token: w.resendActivation(args[0]) };
      case "login": return w.login(...args);
      case "refresh": return w.refresh(args[0]);
      case "logout": return w.logout(args[0]);
      case "listAuctions": { const o = args[0] || {}; return w.listAuctions({ status: o.status, ownerId: o.owner === "me" ? me().id : undefined, limit: o.limit, cursor: o.cursor }); }
      case "getAuction": return w.getAuction(args[0]);
      case "trending": return w.trending(args[0]);
      case "search": return this.#search(args[0], args[1] || {});
      case "suggest": return w.suggest(args[0], args[1]);
      case "createAuction": return w.createAuction(me().id, args[0]);
      case "cancelAuction": return w.cancelAuction(me().id, args[0]);
      case "bids": return w.bidsOf(args[0], args[1] || {});
      case "placeBid": { const [id, amount, key, o] = args; return this.#guardedBid(me().id, id, amount, key, o || {}); }
      case "setProxy": return w.setProxy(me().id, args[0], args[1]);
      case "getProxy": return w.proxyOf(me().id, args[0]);
      case "cancelProxy": return w.cancelProxy(me().id, args[0]);
      case "trustAttacker": {
        const n = (this.attackers = (this.attackers || 0) + 1), pw = uuid();
        const u = w.addUser({ email: `attacker-${Date.now().toString(36)}-${n}@bots.marque.test`, password: pw, name: "Attacker " + n, funds: Math.max(1, Math.round(args[0])), bot: true });
        return { email: u.email, password: pw };
      }
      case "readyz": return this.#readyz();
      case "chaosGet": { w.requireAdmin(me()); return this.#chaos(); }
      case "chaosSet": { w.requireAdmin(me()); return this.#chaosSet(args[0], args[1], args[2]); }
      case "chaosReset": { w.requireAdmin(me()); this.node.setFaults({ ...DEFAULT_FAULTS, rate_limits: this.node.faults.rate_limits }); return this.#chaos(); }
      case "botsGet": return this.#bots();
      case "botsSet": { if (args[0] !== this.rivalsOn) { this.rivalsOn = !!args[0]; if (this.rivalsOn) this.rivalStats.since = Date.now(); } return this.#bots(); }
      case "rateLimits": this.node.setFaults({ ...this.node.faults, rate_limits: !!args[0] }); return { rate_limits: !!args[0] };
      case "wallet": return w.wallet(me().id);
      case "deposit": return w.deposit(me().id, args[0], args[1]);
      case "ledger": return w.ledgerOf(me().id, args[0] || {});
      case "metrics": { w.requireAdmin(me()); return w.metrics({ connections: [...this.connections.values()].reduce((a, b) => a + b, 0), outboxPending: this.pendingEvents + this.kafkaBacklog, kafkaLag: (this.stressLag || 0) + this.kafkaBacklog, kafkaLagParts: this.backlogParts.map((v, i) => v + (i === this.stressPart ? (this.stressLag || 0) : 0)), breakers: { bidding: this.#brState("bidding"), elasticsearch: this.#brState("elasticsearch") } }); }
      case "reconcile": { w.requireAdmin(me()); return w.reconcile(); }
      case "failedOutbox": { w.requireAdmin(me()); return w.failedOutbox(); }
      case "retryOutbox": { w.requireAdmin(me()); return w.retryOutbox(args[0]); }
      case "fastForward": return w.setEndsIn(args[0], (args[1] ?? 90) * 1000);
      case "invariants": return w.invariants(args[0]);
      case "stress": return this.#startStress(args[0]);
      case "connections": this.connections.set(args[0], args[1]); return null;
      case "reset": this.reset(); this.node.bc.postMessage({ t: "reset" }); queueMicrotask(() => location.reload()); return null;
      default: throw new SimError(404, "unknown method " + method);
    }
  }

  /* ---------- simulated faults, breakers and rate limits ---------- */
  #brState(name) {
    const b = this.br[name];
    if (b.state === 2 && Date.now() - b.openedAt >= BREAKER_COOLDOWN) return 1;
    return b.state;
  }
  /** Run one dependency call through a breaker; returns "ok", "fast" (open) or "fail" (counted). */
  #through(name, failing) {
    const b = this.br[name], now = Date.now();
    if (b.state === 2 && now - b.openedAt >= BREAKER_COOLDOWN) b.state = 1;
    if (b.state === 2) return "fast";
    if (failing) {
      b.fails++;
      if (b.fails >= BREAKER_THRESHOLD || b.state === 1) { b.state = 2; b.openedAt = now; }
      return "fail";
    }
    b.state = 0; b.fails = 0;
    return "ok";
  }

  #guardedBid(userId, auctionId, amount, key, o) {
    const f = this.node.faults;
    let rate = null;
    if (f.rate_limits) {
      const now = Date.now(), k = userId;
      const b = this.buckets.get(k) || { tokens: BUCKET.burst, at: now };
      b.tokens = Math.min(BUCKET.burst, b.tokens + (now - b.at) / 1000 * BUCKET.rate); b.at = now;
      if (b.tokens < 1) { this.buckets.set(k, b); throw new SimError(429, "rate limit exceeded", { retry_after: Math.ceil((1 - b.tokens) / BUCKET.rate), rate: { limit: BUCKET.burst, remaining: 0 } }); }
      b.tokens -= 1; this.buckets.set(k, b);
      rate = { limit: BUCKET.burst, remaining: Math.floor(b.tokens) };
    }
    const path = this.#through("bidding", f.bidding);
    if (path === "fast") throw new SimError(503, "bidding is temporarily unavailable, please retry", { breaker: "open", delay_ms: 1 });
    if (path === "fail") throw new SimError(503, "bidding is temporarily unavailable, please retry", { breaker: this.#brState("bidding") === 2 ? "open" : "closed", delay_ms: 450 });
    // bids on one lot arriving within a few ms of each other queue on its row lock, accepted or not
    const nowMs = performance.now(), bq = this.burst;
    this.burst = bq && bq.auctionId === auctionId && nowMs - bq.t < 25 ? { auctionId, t: nowMs, n: bq.n + 1 } : { auctionId, t: nowMs, n: 0 };
    let r;
    try { r = this.world.placeBid(userId, auctionId, amount, key, { ...o, queue: this.burst.n }); }
    catch (e) {
      if (e instanceof SimError && (e.status === 422 || e.status === 409)) {
        const j = (lo, hi) => lo + Math.random() * (hi - lo), w = j(0.9, 2.2) + j(1.1, 2.8);
        e.body.timings = { lock: this.burst.n * w + j(0.1, 0.4), decide: j(0.02, 0.08), write: 0, commit: 0 };
      }
      throw e;
    }
    r.rate = rate;
    const slow = f.postgres_slow ? f.slow_ms : 0;
    if (slow && r.timings) {
      r.timings = { lock: r.timings.lock + slow * 2, decide: r.timings.decide, write: r.timings.write + slow * 5, commit: r.timings.commit + slow };
      r.delay_ms = Math.min(1500, slow * 8);
    }
    if (f.kafka && r.status === 201) { this.backlogParts[partitionOf(auctionId)]++; this.kafkaBacklog++; }
    return r;
  }

  #search(q, o) {
    const f = this.node.faults;
    let mode = "es", delay = 0;
    if (o.force === "postgres") mode = "pg";
    else {
      const path = this.#through("elasticsearch", f.elasticsearch);
      if (path !== "ok") { mode = "pg"; delay = path === "fail" ? 500 : 0; }
    }
    const res = this.world.search(q, { ...o, mode });
    return { ...res, delay_ms: delay + (f.postgres_slow && mode === "pg" ? f.slow_ms * 3 : 0) };
  }

  #bots() {
    const r = this.rivalStats;
    return { enabled: true, simulated: true, ambient: { on: this.rivalsOn, since: this.rivalsOn ? new Date(r.since).toISOString() : undefined, bids: r.bids, refused: r.refused, last_at: r.lastAt ? new Date(r.lastAt).toISOString() : undefined, last_lot: r.lastLot, interval: "every 7 seconds" } };
  }

  #readyz() {
    const f = this.node.faults, st = down => (down ? "failing: injected fault" : "ok");
    return { status: "ok", checks: { postgres: "ok", redis: st(f.redis), elasticsearch: st(f.elasticsearch), kafka: st(f.kafka) }, simulated: true };
  }

  #chaos() {
    const f = this.node.faults;
    return { enabled: true, simulated: true, slow_query_ms: f.slow_ms, faults: FAULTS.map(x => ({ name: x.name, active: !!f[x.name], description: x.does + " " + x.degrades })) };
  }
  #chaosSet(fault, active, slowMs) {
    const f = { ...this.node.faults };
    if (slowMs != null) f.slow_ms = Math.max(0, Math.min(500, Math.round(slowMs)));
    if (fault) {
      if (!FAULTS.some(x => x.name === fault)) throw new SimError(422, "unknown fault");
      f[fault] = !!active;
    }
    this.node.setFaults(f);
    return this.#chaos();
  }

  /* ---------- the stress runner: N bidders fire at one lot at the same instant ---------- */
  #startStress({ auctionId, bidders = 200, rounds = 8 }) {
    const w = this.world, run = uuid();
    w.getAuction(auctionId);
    this.stressPart = partitionOf(auctionId);
    const emit = ev => { const m = { t: "stress", run, ev }; this.node.bc.postMessage(m); this.node.stressEvent(m); };
    setTimeout(() => this.#runStress(run, auctionId, Math.min(300, bidders), Math.min(20, rounds), emit), 120);
    return { id: run };
  }

  async #runStress(run, auctionId, bidders, rounds, emit) {
    const w = this.world, rng = Math.random;
    const bots = Array.from({ length: bidders }, (_, i) => {
      const email = `stress-${String(i + 1).padStart(3, "0")}@bots.marque.test`, id = w.emails.get(email);
      return w.users.get(id) || w.addUser({ email, password: uuid(), name: `Bot ${i + 1}`, funds: 500_000_000 * D, bot: true });
    });
    const a0 = w.auctions.get(auctionId), step = a0.min_increment, start = performance.now();
    let accepted = 0, rejected = 0; const lat = [];
    emit({ type: "progress", round: 0, rounds, accepted, rejected, bidders, elapsed: 0 });
    for (let r = 1; r <= rounds; r++) {
      const a = w.auctions.get(auctionId);
      if (a.status !== "ACTIVE") { emit({ type: "error", error: "The auction closed during the run" }); return; }
      const min = w.minimum(a);                                        // every bot reads this price at once...
      const order = bots.map(b => [rng(), b]).sort((x, y) => x[0] - y[0]).map(x => x[1]);
      this.stressLag = order.length;
      for (let i = 0; i < order.length; i++) {                          // ...and the engine admits them one at a time
        const higher = rng() < 0.2 ? Math.ceil(rng() * 3) * step : 0;
        const t0 = performance.now();
        try { const res = w.placeBid(order[i].id, auctionId, min + higher, "", { queue: i }); accepted++; lat.push(res.timings.lock + res.timings.decide + res.timings.write + res.timings.commit); }
        catch (e) { if (!(e instanceof SimError)) throw e; rejected++; lat.push(performance.now() - t0 + jitter(0.2, 0.8)); }
        this.stressLag = order.length - i - 1;
        if (i % 25 === 24) await sleep(30);                             // let windows paint while the storm runs
      }
      const sorted = [...lat].sort((x, y) => x - y), q = p => sorted[Math.min(sorted.length - 1, Math.floor(p * sorted.length))] || 0;
      const elapsed = (performance.now() - start) / 1000;
      emit({ type: "progress", round: r, rounds, accepted, rejected, bidders, elapsed, p50: q(0.5), p95: q(0.95), perSecond: (accepted + rejected) / Math.max(elapsed, 0.001) });
      await sleep(260);
    }
    this.stressLag = 0;
    const inv = w.invariants(auctionId), a = w.auctions.get(auctionId);
    emit({ type: "done", accepted, rejected, bidders, rounds, elapsed: (performance.now() - start) / 1000, finalBid: a.current_bid, bidCount: a.bid_count, invariants: inv });
  }
}

/* ================================================================== per-window node */
class Node {
  constructor() {
    this.role = "pending"; this.host = null; this.pending = new Map(); this.stressHandlers = new Map();
    this.faults = { ...DEFAULT_FAULTS };
    this.bc = new BroadcastChannel(CHANNEL);
    this.feeds = new Set();
    this.ready = new Promise(r => (this.markReady = r));
    this.bc.onmessage = e => this.#on(e.data);
    this.id = uuid();
    if (navigator.locks) navigator.locks.request(LOCK, async () => { await this.#lead(); await new Promise(() => {}); });
    else this.#lead();
    setTimeout(() => { if (this.role === "pending") { this.role = "follower"; this.bc.postMessage({ t: "who" }); this.markReady(); } }, 140);
  }

  async #lead() {
    const host = new Host(this);
    await host.start();
    this.host = host; this.role = "leader";
    this.bc.postMessage({ t: "leader" });
    this.markReady();
  }

  #on(m) {
    switch (m.t) {
      case "rpc": if (this.role === "leader") this.#serve(m); break;
      case "res": { const p = this.pending.get(m.id); if (p) { this.pending.delete(m.id); m.ok ? p.res(m.result) : p.rej(m.error); } break; }
      case "event": this.deliver(m.ev); break;
      case "stress": this.stressEvent(m); break;
      case "who": if (this.role === "leader") { this.bc.postMessage({ t: "leader" }); this.bc.postMessage({ t: "faults", faults: this.faults }); } break;
      case "faults": this.faults = m.faults; break;
      case "reset": location.reload(); break;
      default: break;
    }
  }

  async #serve(m) {
    let reply;
    try { reply = { t: "res", id: m.id, ok: true, result: await this.host.dispatch(m.method, m.args, m.token) }; }
    catch (e) { reply = { t: "res", id: m.id, ok: false, error: e instanceof SimError ? { status: e.status, body: e.body } : { status: 500, body: { error: String(e.message || e) } } }; }
    this.bc.postMessage(reply);
  }

  setFaults(f) { this.faults = f; this.bc.postMessage({ t: "faults", faults: f }); }
  deliver(ev) { this.feeds.forEach(f => f._deliver(ev)); }
  stressEvent(m) { this.stressHandlers.get(m.run)?.(m.ev); }

  async call(method, args = [], token = null) {
    await this.ready;
    const asError = e => new ApiError(e.status, e.body, null);
    if (this.role === "leader") {
      try { return this.host.dispatch(method, args, token); } catch (e) { if (e instanceof SimError) throw asError(e); throw e; }
    }
    for (let attempt = 0; attempt < 4; attempt++) {
      const id = uuid();
      const p = new Promise((res, rej) => this.pending.set(id, { res, rej }));
      this.bc.postMessage({ t: "rpc", id, method, args, token });
      const r = await Promise.race([p.then(v => ({ v }), e => ({ e })), sleep(1500).then(() => null)]);
      this.pending.delete(id);
      if (r === null) { if (this.role === "leader") return this.call(method, args, token); continue; }
      if (r.e) throw asError(r.e);
      return r.v;
    }
    throw new ApiError(503, { error: "The demo engine is not answering. Reload the page." }, null);
  }
}

/* ================================================================== the feed, same shape as feed.js */
class SimFeed extends Emitter {
  state = "open"; clockOffset = 0;
  #subs = new Map(); #versions = new Map();
  constructor(node) { super(); this.node = node; node.feeds.add(this); }
  start() { return this; }
  serverNow() { return Date.now(); }
  subscribe(id, fn) {
    let set = this.#subs.get(id);
    if (!set) { set = new Set(); this.#subs.set(id, set); this.#report(); }
    set.add(fn);
    return () => { set.delete(fn); if (!set.size) { this.#subs.delete(id); this.#versions.delete(id); this.#report(); } };
  }
  #report() { this.node.call("connections", [this.node.id, this.#subs.size > 0 ? 1 : 0]).catch(() => {}); }
  _deliver(ev) {
    const id = ev.auction.id, v = ev.auction.version;
    if (v <= (this.#versions.get(id) ?? -1)) return;
    if (this.#subs.has(id)) this.#versions.set(id, v);
    const meta = { receivedAt: performance.now(), wallAt: Date.now() };
    this.#subs.get(id)?.forEach(fn => { try { fn(ev, meta); } catch (e) { console.error(e); } });
    this.emit("update", ev, meta);
  }
}

/* ================================================================== the backend */
export function createSim() {
  const node = new Node();
  const session = new Session("sim");
  const feed = new SimFeed(node);
  const call = async (method, args = [], { auth = false } = {}) => {
    const run = () => node.call(method, args, auth ? session.access : null);
    await sleep((method === "placeBid" ? jitter(2, 6) : jitter(10, 35)) + (node.faults.postgres_slow ? node.faults.slow_ms * 2 : 0));
    try {
      const r = await run();
      if (r && r.delay_ms) await sleep(r.delay_ms);
      return r;
    } catch (e) {
      if (e instanceof ApiError && e.body?.delay_ms) await sleep(e.body.delay_ms);
      if (e instanceof ApiError && e.status === 401 && auth && session.refresh) {
        try { session.set(await node.call("refresh", [session.refresh])); return await run(); } catch { session.clear(); }
      }
      throw e;
    }
  };
  const authed = (method, ...args) => call(method, args, { auth: true });

  const be = {
    kind: "sim",
    label: "Demo engine",
    detail: "runs in this browser",
    base: "", session, feed, node,
    caps: { stress: true, async: false, serverTiming: true, fastForward: true, simulated: true },
    async ready() { await node.ready; },

    me: () => session.user,
    onAuth: fn => session.on("change", fn),
    async register(email, password) {
      const r = await call("register", [email, password]);
      return { user: r.user, activation: { token: r.activationToken } };
    },
    activate: token => call("activate", [token]),
    async resendActivation(email) { return (await call("resend", [email])).token; },
    async login(email, password) {
      const r = await call("login", [email, password]);
      session.set(r, r.user);
      return session.user;
    },
    async logout() { const t = session.refresh; session.clear(); if (t) await node.call("logout", [t]).catch(() => {}); },

    listAuctions: (o = {}) => (o.owner ? authed("listAuctions", o) : call("listAuctions", [o])),
    async allAuctions(o = {}, maxPages = 5) {
      const out = []; let cursor;
      for (let i = 0; i < maxPages; i++) { const p = await be.listAuctions({ ...o, limit: 100, cursor }); out.push(...p.auctions); if (!p.next_cursor) break; cursor = p.next_cursor; }
      return out;
    },
    getAuction: id => call("getAuction", [id]),
    trending: limit => call("trending", [limit]),
    search: (q, o) => call("search", [q, o]),
    suggest: (q, limit) => call("suggest", [q, limit]),
    createAuction: input => authed("createAuction", input),
    cancelAuction: id => authed("cancelAuction", id),
    bids: (id, o) => call("bids", [id, o]),

    async placeBid(auctionId, amount, { key = uuid() } = {}) {
      const base = { key, amount, auctionId };
      const sentAt = Date.now(), t0 = performance.now();
      try {
        const r = await authed("placeBid", auctionId, amount, key);
        const ms = performance.now() - t0, tm = r.timings;
        return { ...base, ok: true, status: r.status, data: r.body, replayed: r.replayed, queued: false, requestId: hex(24), traceId: null, rate: r.rate || null,
          serverTiming: tm ? { ...tm, bidding: tm.lock + tm.decide + tm.write + tm.commit + 0.2 } : { bidding: 0.3 }, sentAt, ms, t0 };
      } catch (e) {
        if (!(e instanceof ApiError)) throw e;
        return { ...base, ok: false, status: e.status, error: e, data: e.body, replayed: false, requestId: hex(24), traceId: null, rate: e.body?.rate || null, serverTiming: e.body?.timings || null, sentAt, ms: performance.now() - t0, t0 };
      }
    },
    /**
     * Trust page: a throwaway account holding exactly `budget` cents, so a burst of bids can
     * be shown to stop at the money. The real API cannot mint accounts, so there the page
     * uses the signed-in person's own wallet (see live.js).
     */
    trust: {
      async attacker(budget) {
        const c = await node.call("trustAttacker", [budget]);
        const r = await node.call("login", [c.email, c.password]);
        const token = r.access_token;
        return {
          label: "a new account",
          userId: r.user.id,
          wallet: () => node.call("wallet", [], token),
          async placeBid(auctionId, amount, key = uuid()) {
            const sentAt = Date.now(), t0 = performance.now();
            await sleep(jitter(2, 6));
            try {
              const x = await node.call("placeBid", [auctionId, amount, key], token);
              if (x.delay_ms) await sleep(x.delay_ms);
              return { key, ok: true, status: x.status, data: x.body, replayed: x.replayed, rate: x.rate || null, serverTiming: x.timings, sentAt, ms: performance.now() - t0, t0 };
            } catch (e) {
              if (!(e instanceof ApiError)) throw e;
              if (e.body?.delay_ms) await sleep(e.body.delay_ms);
              return { key, ok: false, status: e.status, error: e, data: e.body, replayed: false, rate: e.body?.rate || null, serverTiming: e.body?.timings || null, sentAt, ms: performance.now() - t0, t0 };
            }
          },
        };
      },
    },

    /* demo bots: in the demo engine the rival bidders run in this browser */
    bots: {
      get: () => call("botsGet"),
      setAmbient: on => call("botsSet", [!!on]),
    },

    /* maximum (proxy) bids */
    setProxy: (auctionId, max) => authed("setProxy", auctionId, max),
    async getProxy(auctionId) { try { return await authed("getProxy", auctionId); } catch (e) { if (e.status === 404) return null; throw e; } },
    cancelProxy: auctionId => authed("cancelProxy", auctionId),

    /* the operator's fault switchboard (simulated here; /v1/admin/chaos on the real API) */
    chaos: {
      get: () => authed("chaosGet"),
      set: (fault, active, o = {}) => authed("chaosSet", fault, active, o.slowMs ?? null),
      reset: () => authed("chaosReset"),
      setRateLimits: on => call("rateLimits", [on]),
    },
    bidRequest: () => Promise.reject(new ApiError(404, { error: "Not available in the demo engine" }, null)),

    wallet: () => authed("wallet"),
    deposit: (amount, key = uuid()) => authed("deposit", amount, key),
    ledger: o => authed("ledger", o),

    readyz: () => call("readyz"),
    livez: async () => ({ ok: true, ms: 0.4 }),
    metrics: () => authed("metrics"),
    reconcile: () => authed("reconcile"),
    failedOutbox: () => authed("failedOutbox"),
    retryOutbox: id => authed("retryOutbox", id),
    invariants: id => call("invariants", [id]),
    fastForward: (id, secs = 90) => call("fastForward", [id, secs]),
    reset: () => call("reset"),

    stress: {
      available: async () => true,
      async start(opts, onEvent) {
        const { id } = await call("stress", [opts]);
        node.stressHandlers.set(id, ev => { onEvent(ev); if (ev.type === "done" || ev.type === "error") node.stressHandlers.delete(id); });
        return { stop: () => node.stressHandlers.delete(id) };
      },
    },
  };
  return be;
}
