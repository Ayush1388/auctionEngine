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

const SAVE_KEY = "marque-sim-world-v2";
const LOCK = "marque-sim-host";
const CHANNEL = "marque-sim";
const PASSWORD = DEMO_PASSWORD;
const jitter = (lo, hi) => lo + Math.random() * (hi - lo);
const hex = n => Array.from({ length: n }, () => (Math.random() * 16 | 0).toString(16)).join("");
const D = 100;                       // dollars to cents


/* ================================================================== host */
class Host {
  constructor(node) { this.node = node; this.world = null; this.stress = new Map(); this.pendingEvents = 0; this.connections = new Map(); this.timers = []; }

  async start() {
    const saved = local.json(SAVE_KEY);
    try { if (saved && saved.v === 1) this.world = World.fromJSON(saved); } catch { this.world = null; }
    if (!this.world) { this.world = new World(); await this.#seed(); this.save(); }
    this.rivals = [...this.world.users.values()].filter(u => u.bot && u.email.startsWith("bidder-"));
    this.world.onEvent(ev => this.#publish(ev));
    this.timers.push(setInterval(() => this.world.tick(), 1000));
    this.timers.push(setInterval(() => this.#rival(), 7000));
    this.timers.push(setInterval(() => this.save(), 4000));
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
    const w = this.world, live = [...w.auctions.values()].filter(a => a.status === "ACTIVE" && Date.parse(a.ends_at) - Date.now() > 20000 && !a.item.name.startsWith("Stress"));
    if (!live.length) return;
    const a = live[Math.floor(Math.random() * live.length)];
    const who = this.rivals.filter(r => r.id !== a.current_bidder_id)[Math.floor(Math.random() * (this.rivals.length - 1))];
    try { w.placeBid(who.id, a.id, w.minimum(a), "", { queue: 0 }); } catch { /* a rival that can't afford it just skips */ }
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
      case "search": return w.search(args[0], args[1] || {});
      case "suggest": return w.suggest(args[0], args[1]);
      case "createAuction": return w.createAuction(me().id, args[0]);
      case "cancelAuction": return w.cancelAuction(me().id, args[0]);
      case "bids": return w.bidsOf(args[0], args[1] || {});
      case "placeBid": { const [id, amount, key, o] = args; return w.placeBid(me().id, id, amount, key, o || {}); }
      case "wallet": return w.wallet(me().id);
      case "deposit": return w.deposit(me().id, args[0], args[1]);
      case "ledger": return w.ledgerOf(me().id, args[0] || {});
      case "metrics": { w.requireAdmin(me()); return w.metrics({ connections: [...this.connections.values()].reduce((a, b) => a + b, 0), outboxPending: this.pendingEvents, kafkaLag: this.stressLag || 0 }); }
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

  /* ---------- the stress runner: N bidders fire at one lot at the same instant ---------- */
  #startStress({ auctionId, bidders = 200, rounds = 8 }) {
    const w = this.world, run = uuid();
    w.getAuction(auctionId);
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
      case "who": if (this.role === "leader") this.bc.postMessage({ t: "leader" }); break;
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
    await sleep(method === "placeBid" ? jitter(2, 6) : jitter(10, 35));
    try { return await run(); }
    catch (e) {
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
        return { ...base, ok: true, status: r.status, data: r.body, replayed: r.replayed, queued: false, requestId: hex(24), traceId: null,
          serverTiming: tm ? { ...tm, bidding: tm.lock + tm.decide + tm.write + tm.commit + 0.2 } : { bidding: 0.3 }, sentAt, ms, t0 };
      } catch (e) {
        if (!(e instanceof ApiError)) throw e;
        return { ...base, ok: false, status: e.status, error: e, data: e.body, replayed: false, requestId: hex(24), traceId: null, serverTiming: null, sentAt, ms: performance.now() - t0, t0 };
      }
    },
    bidRequest: () => Promise.reject(new ApiError(404, { error: "Not available in the demo engine" }, null)),

    wallet: () => authed("wallet"),
    deposit: (amount, key = uuid()) => authed("deposit", amount, key),
    ledger: o => authed("ledger", o),

    readyz: async () => ({ status: "ok", checks: { postgres: "ok", redis: "ok", elasticsearch: "ok", kafka: "ok" }, simulated: true }),
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
