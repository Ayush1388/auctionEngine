// The demo engine's core: an in-memory auction house that follows the same rules
// as the Go backend (internal/bidding, internal/wallet, internal/auction).
//
// It exists so the site works with no server at all (a hosted demo, a quick
// look) and so the same UI can be tested without Docker. Everything here is
// plain data in, plain data out: no DOM, no timers, no network. sim.js adds
// those. Rules mirrored from the backend:
//
//   - minimum bid: the starting price, then current + increment
//   - one leader; the leader's money is reserved; the old leader's is released
//   - raising your own bid reserves only the difference
//   - a bid inside the last 2 minutes moves the end to now + 2 minutes (max 10 times)
//   - the same Idempotency-Key returns the original bid instead of bidding twice
//   - every money movement is a journal whose lines sum to zero
//   - ACTIVE -> COMPLETED at ends_at, then the winner's reserve pays the seller
import { uuid } from "./util.js";
import { SNIPE_WINDOW, MAX_EXTENSIONS, MIN_DURATION, MAX_DURATION, MAX_START_DELAY } from "./rules.js";

export { SNIPE_WINDOW, MAX_EXTENSIONS, MIN_DURATION, MAX_DURATION, MAX_START_DELAY };
export const MAX_DEPOSIT = 1_000_000_000;   // the backend's wallet.MaxDeposit: $10,000,000 a deposit
export const ACCESS_TTL = 15 * 60 * 1000;

export class SimError extends Error {
  constructor(status, message, extra = {}) { super(message); this.status = status; this.body = { error: message, ...extra }; }
}

const b64 = s => btoa(unescape(encodeURIComponent(s))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
const fromB64 = s => decodeURIComponent(escape(atob(s.replace(/-/g, "+").replace(/_/g, "/"))));
const iso = t => new Date(t).toISOString();

export class World {
  constructor({ now = () => Date.now(), rng = Math.random } = {}) {
    this.now = now;
    this.rng = rng;
    this.users = new Map();          // id -> user
    this.emails = new Map();         // email -> id
    this.wallets = new Map();        // userId -> { available, reserved }
    this.ledger = [];                // { id, transaction_id, user_id, kind, account, amount, auction_id, created_at }
    this.seq = 0;
    this.auctions = new Map();       // id -> internal auction
    this.order = [];                 // auction ids, oldest first
    this.bidLog = [];                // every accepted bid, in commit order
    this.idem = new Map();           // `${user}:${key}` -> { auctionId, amount, bidId }
    this.depositKeys = new Map();    // `${user}:${key}` -> wallet
    this.refreshTokens = new Map();  // token -> { userId, used }
    this.deadLetters = [];
    this.listeners = new Set();
    this.counters = { bids: {}, durations: [], settled: 0, events: 0 };
  }

  /* ------------------------------------------------------------ events */
  onEvent(fn) { this.listeners.add(fn); return () => this.listeners.delete(fn); }
  #emit(ev) { this.counters.events++; this.listeners.forEach(fn => fn(ev)); }

  /* ------------------------------------------------------------ users and auth */
  /** Create an account directly (seeding). Returns the user. */
  addUser({ email, password = "marque-demo-password", role = "user", activated = true, funds = 0, bot = false, name }) {
    const id = uuid();
    const u = { id, email, password, role, name: name || email.split("@")[0], bot, activated_at: activated ? iso(this.now()) : null, token: null, created_at: iso(this.now()) };
    this.users.set(id, u); this.emails.set(email.toLowerCase(), id);
    this.wallets.set(id, { available: 0, reserved: 0 });
    for (let i = 0, left = funds; left > 0; i++) { const part = Math.min(left, MAX_DEPOSIT); this.deposit(id, part, `seed-${id}-${i}`); left -= part; }   // one deposit is capped, like the API
    return u;
  }

  register(email, password) {
    const fields = {};
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email || "")) fields.email = "must be a valid email address";
    if (!password || password.length < 15) fields.password = "must be at least 15 characters";
    else if (password.length > 128) fields.password = "must be at most 128 characters";
    if (Object.keys(fields).length) throw new SimError(400, "invalid input", { fields });
    if (this.emails.has(email.toLowerCase())) throw new SimError(409, "email already exists");
    const u = this.addUser({ email, password, activated: false });
    u.token = "act_" + uuid().replace(/-/g, "");
    return { user: { id: u.id, email: u.email }, activationToken: u.token };
  }

  activate(token) {
    const u = [...this.users.values()].find(x => x.token && x.token === token && !x.activated_at);
    if (!u) throw new SimError(400, "invalid or expired activation token");
    u.activated_at = iso(this.now()); u.token = null;
    return { message: "account activated" };
  }

  resendActivation(email) {
    const u = this.users.get(this.emails.get((email || "").toLowerCase()));
    if (u && !u.activated_at) u.token = "act_" + uuid().replace(/-/g, "");
    return u && !u.activated_at ? u.token : null;
  }

  #issue(u) {
    const exp = Math.floor((this.now() + ACCESS_TTL) / 1000);
    const access = ["e30", b64(JSON.stringify({ user_id: u.id, role: u.role, sub: u.id, iss: "marque-demo", exp })), "demo"].join(".");
    const refresh = "simr_" + uuid();
    this.refreshTokens.set(refresh, { userId: u.id, used: false });
    return { access_token: access, token_type: "Bearer", refresh_token: refresh, refresh_expires_at: iso(this.now() + 30 * 24 * 3600 * 1000) };
  }

  login(email, password) {
    const u = this.users.get(this.emails.get((email || "").toLowerCase()));
    if (!u || u.password !== password) throw new SimError(401, "invalid email or password");
    if (!u.activated_at) throw new SimError(403, "account is not activated");
    return { ...this.#issue(u), user: { id: u.id, email: u.email, activated_at: u.activated_at } };
  }

  /** Swap a refresh token for new tokens. A token used twice revokes the whole session, like the server. */
  refresh(refreshToken) {
    const r = this.refreshTokens.get(refreshToken);
    if (!r) throw new SimError(401, "invalid, expired or reused refresh token");
    if (r.used) { for (const [t, v] of this.refreshTokens) if (v.userId === r.userId) this.refreshTokens.delete(t); throw new SimError(401, "invalid, expired or reused refresh token"); }
    r.used = true;
    return this.#issue(this.users.get(r.userId));
  }

  logout(refreshToken) { this.refreshTokens.delete(refreshToken); }

  /** Resolve a bearer token to a user, or throw 401. */
  authenticate(token) {
    try {
      const c = JSON.parse(fromB64(String(token).split(".")[1]));
      const u = this.users.get(c.user_id);
      if (u && c.exp * 1000 > this.now()) return u;
    } catch { /* fall through */ }
    throw new SimError(401, "unauthorized");
  }
  requireAdmin(u) { if (u.role !== "admin") throw new SimError(403, "forbidden"); }

  /* ------------------------------------------------------------ wallet and ledger */
  wallet(userId) {
    const w = this.wallets.get(userId) || { available: 0, reserved: 0 };
    return { available: w.available, reserved: w.reserved, total: w.available + w.reserved };
  }

  /** One journal: lines [{user, account, amount, kind, auction}] that must sum to zero. */
  #post(lines) {
    if (lines.reduce((s, l) => s + l.amount, 0) !== 0) throw new SimError(500, "ledger journal does not balance");
    const transaction_id = uuid(), created_at = iso(this.now());
    for (const l of lines) {
      this.ledger.push({ id: ++this.seq, transaction_id, user_id: l.user, kind: l.kind, account: l.account, amount: l.amount, auction_id: l.auction || null, created_at });
      if (l.user) { const w = this.wallets.get(l.user); w[l.account] += l.amount; }
    }
  }

  deposit(userId, amount, key) {
    if (!key) throw new SimError(400, "idempotency key is required");
    const k = userId + ":" + key;
    if (this.depositKeys.has(k)) return this.wallet(userId);
    if (!Number.isInteger(amount) || amount <= 0) throw new SimError(422, "amount must be positive");
    if (amount > MAX_DEPOSIT) throw new SimError(422, "amount is above the deposit limit");
    this.#post([{ user: userId, account: "available", amount, kind: "DEPOSIT" }, { user: null, account: "external", amount: -amount, kind: "DEPOSIT" }]);
    this.depositKeys.set(k, true);
    return this.wallet(userId);
  }

  ledgerOf(userId, { limit = 50, before } = {}) {
    const mine = this.ledger.filter(e => e.user_id === userId && (before == null || e.id < before)).sort((a, b) => b.id - a.id);
    const page = mine.slice(0, limit);
    return { entries: page.map(({ user_id, ...e }) => e), next_before: mine.length > limit ? page[page.length - 1].id : null };
  }

  /** Recompute every balance from the ledger and check each journal sums to zero. */
  reconcile() {
    const sums = new Map(), journals = new Map();
    let external = 0;
    for (const e of this.ledger) {
      journals.set(e.transaction_id, (journals.get(e.transaction_id) || 0) + e.amount);
      if (!e.user_id) { external += e.amount; continue; }
      const s = sums.get(e.user_id) || { available: 0, reserved: 0 };
      s[e.account] += e.amount; sums.set(e.user_id, s);
    }
    let mismatches = 0, walletTotal = 0, reservedTotal = 0;
    for (const [id, w] of this.wallets) {
      const s = sums.get(id) || { available: 0, reserved: 0 };
      if (s.available !== w.available || s.reserved !== w.reserved) mismatches++;
      walletTotal += w.available + w.reserved; reservedTotal += w.reserved;
    }
    let active = 0;
    for (const a of this.auctions.values()) if (a.status === "ACTIVE" && a.current_bid != null) active += a.current_bid;
    const unbalanced = [...journals.values()].filter(v => v !== 0).length;
    return { ok: !mismatches && !unbalanced && reservedTotal === active, mismatches, unbalanced_journals: unbalanced, money_deposited: -external, money_in_wallets: walletTotal, reserved_in_wallets: reservedTotal, active_reservations_total: active };
  }

  /* ------------------------------------------------------------ auctions */
  #public(a) {
    const { version, bids, ...rest } = a;      // the REST shape has neither
    return { ...rest, item: { ...a.item } };
  }
  #snapshot(a) {
    return { id: a.id, status: a.status, current_bid: a.current_bid, current_bidder_id: a.current_bidder_id, bid_count: a.bid_count, min_next_bid: this.minimum(a), ends_at: a.ends_at, extensions: a.extensions, version: a.version };
  }
  minimum(a) { return a.current_bid == null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment; }

  createAuction(ownerId, input, { seed = false } = {}) {
    const fields = {};
    const it = input?.item || {};
    if (!it.name) fields["item.name"] = "is required"; else if (it.name.length > 200) fields["item.name"] = "must be at most 200 characters";
    if (!it.type) fields["item.type"] = "is required"; else if (it.type.length > 50) fields["item.type"] = "must be at most 50 characters";
    if ((it.description || "").length > 5000) fields["item.description"] = "must be at most 5000 characters";
    if (!Number.isInteger(input?.starting_price) || input.starting_price < 0) fields.starting_price = "must be zero or more";
    if (!Number.isInteger(input?.min_increment ?? 0) || (input?.min_increment ?? 0) < 0) fields.min_increment = "must be zero or more";
    const now = this.now(), s = Date.parse(input?.starts_at), e = Date.parse(input?.ends_at);
    if (!input?.starts_at || Number.isNaN(s)) fields.starts_at = "is required";
    else if (!seed && s <= now) fields.starts_at = "must be in the future";
    else if (s > now + MAX_START_DELAY) fields.starts_at = "must be within 90 days";
    if (!input?.ends_at || Number.isNaN(e)) fields.ends_at = "is required";
    else if (!fields.starts_at && e <= s) fields.ends_at = "must be after starts_at";
    else if (!fields.starts_at && e - s < MIN_DURATION) fields.ends_at = "must be at least 1 minute after starts_at";
    else if (!fields.starts_at && e - s > MAX_DURATION) fields.ends_at = "must be at most 30 days after starts_at";
    if (Object.keys(fields).length) throw new SimError(400, "invalid input", { fields });

    const a = {
      id: uuid(), owner_id: ownerId, item: { id: uuid(), name: it.name, type: it.type, description: it.description || "" },
      starting_price: input.starting_price, min_increment: Math.max(input.min_increment || 0, 1),
      current_bid: null, current_bidder_id: null, bid_count: 0, extensions: 0, settled_at: null,
      starts_at: iso(s), ends_at: iso(e), status: s <= now ? "ACTIVE" : "NOT_ACTIVE",
      created_at: iso(now), updated_at: iso(now), version: 1, bids: [],
    };
    this.auctions.set(a.id, a); this.order.push(a.id);
    this.#emit({ type: "auction.updated", cause: "auction.created", auction: this.#snapshot(a) });
    return this.#public(a);
  }

  #get(id) { const a = this.auctions.get(id); if (!a) throw new SimError(404, "auction not found"); return a; }
  getAuction(id) { return this.#public(this.#get(id)); }

  cancelAuction(userId, id) {
    const a = this.#get(id);
    if (a.owner_id !== userId) throw new SimError(403, "not allowed to modify this auction");
    if (a.status !== "NOT_ACTIVE" || Date.parse(a.starts_at) <= this.now()) throw new SimError(409, "auction has already started");
    a.status = "CANCELLED"; a.version++; a.updated_at = iso(this.now());
    this.#emit({ type: "auction.updated", cause: "auction.cancelled", auction: this.#snapshot(a) });
    return this.#public(a);
  }

  listAuctions({ status, ownerId, limit = 20, cursor } = {}) {
    let all = this.order.map(id => this.auctions.get(id)).reverse();
    if (status) all = all.filter(a => a.status === status);
    if (ownerId) all = all.filter(a => a.owner_id === ownerId);
    const from = cursor ? Number(cursor) || 0 : 0;
    const page = all.slice(from, from + limit);
    return { auctions: page.map(a => this.#public(a)), next_cursor: from + limit < all.length ? String(from + limit) : null };
  }

  trending(limit = 10) {
    const cutoff = this.now() - 3600 * 1000;
    return [...this.auctions.values()].filter(a => a.status === "ACTIVE")
      .map(a => ({ a, n: a.bids.filter(b => Date.parse(b.created_at) >= cutoff).length }))
      .sort((x, y) => y.n - x.n || y.a.bid_count - x.a.bid_count).slice(0, limit).map(x => this.#public(x.a));
  }

  /** Typo-tolerant search over title and description (one edit per word), like the Elasticsearch path. */
  search(q, { status, limit = 20, cursor } = {}) {
    const terms = String(q || "").toLowerCase().split(/\s+/).filter(Boolean);
    if (!terms.length) throw new SimError(400, "q is required");
    const near = (w, t) => w.includes(t) || (t.length > 3 && w.length > 2 && editsWithin1(w, t));
    const score = a => {
      const words = (a.item.name + " " + a.item.description).toLowerCase().split(/[^a-z0-9]+/).filter(Boolean);
      let s = 0;
      for (const t of terms) {
        if (a.item.name.toLowerCase().includes(t)) s += 3; else if (words.some(w => near(w, t))) s += 1; else return 0;
      }
      return s;
    };
    let hits = [...this.auctions.values()].filter(a => !status || a.status === status).map(a => ({ a, s: score(a) })).filter(x => x.s > 0).sort((x, y) => y.s - x.s);
    const from = cursor ? Number(cursor) || 0 : 0;
    const page = hits.slice(from, from + limit);
    return { auctions: page.map(x => ({ ...this.#public(x.a), highlight: highlight(x.a.item.name, terms) })), next_cursor: from + limit < hits.length ? String(from + limit) : null, backend: "demo" };
  }

  suggest(q, limit = 5) {
    const t = String(q || "").toLowerCase();
    if (t.length < 2) throw new SimError(400, "q must be at least 2 characters");
    return [...this.auctions.values()].filter(a => a.status === "ACTIVE" && a.item.name.toLowerCase().split(/\s+/).some(w => w.startsWith(t)) || (a.status === "ACTIVE" && a.item.name.toLowerCase().includes(t)))
      .slice(0, limit).map(a => ({ id: a.id, title: a.item.name }));
  }

  bidsOf(auctionId, { limit = 50, cursor } = {}) {
    const a = this.#get(auctionId);
    const all = [...a.bids].reverse();
    const from = cursor ? Number(cursor) || 0 : 0;
    const page = all.slice(from, from + limit);
    return { bids: page.map(b => ({ ...b })), next_cursor: from + limit < all.length ? String(from + limit) : null };
  }

  /* ------------------------------------------------------------ bidding */
  /**
   * Place a bid. Returns { status, body, timings, replayed } or throws SimError.
   * `at` back-dates a seeded bid; `queue` is how many bids were ahead on this
   * auction in the same instant (it lengthens the simulated lock wait).
   */
  placeBid(userId, auctionId, amount, key = "", { at, queue = 0 } = {}) {
    const t = at ?? this.now();
    const result = (label, fn) => { try { const r = fn(); this.#count(r.replayed ? "replayed" : "accepted"); return r; } catch (e) { this.#count(label(e)); throw e; } };
    return result(e => (e instanceof SimError ? outcome(e) : "error"), () => {
      if (key.length > 255) throw new SimError(422, "idempotency key must be at most 255 characters");
      const a = this.#get(auctionId);

      // idempotency is checked after the "lock", like the server
      const prior = key && this.idem.get(userId + ":" + key);
      if (prior) {
        if (prior.auctionId !== auctionId || prior.amount !== amount) throw new SimError(422, "idempotency key was already used for a different bid");
        const b = a.bids.find(x => x.id === prior.bidId);
        return { status: 200, replayed: true, timings: null, body: { bid: { ...b }, current_bid: a.current_bid, bid_count: a.bid_count, ends_at: a.ends_at, extended: false, replayed: true } };
      }

      // decide
      if (!Number.isInteger(amount) || amount <= 0) throw new SimError(422, "bid amount must be positive");
      if (a.status === "COMPLETED") throw new SimError(409, "auction has ended");
      if (a.status !== "ACTIVE" || t < Date.parse(a.starts_at)) throw new SimError(409, "auction is not accepting bids");
      if (t >= Date.parse(a.ends_at)) throw new SimError(409, "auction has ended");
      if (userId === a.owner_id) throw new SimError(403, "sellers cannot bid on their own auction");
      const min = this.minimum(a);
      if (amount < min) throw new SimError(422, `bid must be at least ${min}`, { minimum_amount: min });

      const prevBidder = a.current_bidder_id, prevAmount = a.current_bid;
      const reserve = prevBidder === userId ? amount - prevAmount : amount;
      const w = this.wallets.get(userId);
      if (!w || w.available < reserve) throw new SimError(422, "insufficient funds");

      const endsMs = Date.parse(a.ends_at);
      const extended = endsMs - t < SNIPE_WINDOW && a.extensions < MAX_EXTENSIONS;

      // write: release the old leader, reserve the new one, update the auction, all or nothing
      if (prevBidder && prevBidder !== userId) this.#post([{ user: prevBidder, account: "reserved", amount: -prevAmount, kind: "RELEASE", auction: a.id }, { user: prevBidder, account: "available", amount: prevAmount, kind: "RELEASE", auction: a.id }]);
      this.#post([{ user: userId, account: "available", amount: -reserve, kind: "RESERVE", auction: a.id }, { user: userId, account: "reserved", amount: reserve, kind: "RESERVE", auction: a.id }]);
      const bid = { id: uuid(), auction_id: a.id, user_id: userId, amount, created_at: iso(t) };
      a.bids.push(bid); this.bidLog.push(bid);
      a.current_bid = amount; a.current_bidder_id = userId; a.bid_count++; a.version++; a.updated_at = iso(t);
      if (extended) { a.ends_at = iso(t + SNIPE_WINDOW); a.extensions++; }
      if (key) this.idem.set(userId + ":" + key, { auctionId, amount, bidId: bid.id });

      const timings = this.#timings(queue);
      this.counters.durations.push(timings.lock + timings.decide + timings.write + timings.commit);
      if (this.counters.durations.length > 2000) this.counters.durations.splice(0, 1000);
      this.#emit({
        type: "auction.updated", cause: "bid.placed", auction: this.#snapshot(a),
        bid: { id: bid.id, bidder_id: userId, previous_bidder_id: prevBidder, amount, extended },
        timing: { placed_at: iso(t), sent_at: iso(t) },
      });
      return { status: 201, replayed: false, timings, body: { bid: { ...bid }, current_bid: amount, bid_count: a.bid_count, ends_at: a.ends_at, extended, replayed: false } };
    });
  }

  /** Simulated phase costs in ms: small for a quiet auction, longer when bids queue on the row lock. */
  #timings(queue) {
    const j = (lo, hi) => lo + this.rng() * (hi - lo);
    const write = j(0.9, 2.2), commit = j(1.1, 2.8);
    return { lock: queue ? queue * (write + commit) * j(0.7, 1) + j(0.1, 0.4) : j(0.08, 0.35), decide: j(0.02, 0.08), write, commit };
  }

  #count(label) { this.counters.bids[label] = (this.counters.bids[label] || 0) + 1; }

  /** Demo only: move an auction's close so the last minutes can be seen without waiting. */
  setEndsIn(id, ms) {
    const a = this.#get(id);
    if (a.status !== "ACTIVE") throw new SimError(409, "auction is not accepting bids");
    a.ends_at = iso(this.now() + ms); a.version++; a.updated_at = iso(this.now());
    this.#emit({ type: "auction.updated", cause: "demo.fast_forward", auction: this.#snapshot(a) });
    return this.#public(a);
  }

  /* ------------------------------------------------------------ lifecycle */
  /** Start due auctions, close finished ones and pay out. Safe to call as often as you like. */
  tick() {
    const now = this.now();
    for (const a of this.auctions.values()) {
      if (a.status === "NOT_ACTIVE" && now >= Date.parse(a.starts_at)) {
        a.status = "ACTIVE"; a.version++; a.updated_at = iso(now);
        this.#emit({ type: "auction.updated", cause: "auction.activated", auction: this.#snapshot(a) });
      }
      if (a.status === "ACTIVE" && now >= Date.parse(a.ends_at)) {
        a.status = "COMPLETED"; a.version++; a.updated_at = iso(now);
        this.#emit({ type: "auction.updated", cause: "auction.completed", auction: this.#snapshot(a) });
        if (a.current_bidder_id && !a.settled_at) this.#settle(a, now);
      }
    }
  }

  #settle(a, now) {
    this.#post([
      { user: a.current_bidder_id, account: "reserved", amount: -a.current_bid, kind: "SETTLE", auction: a.id },
      { user: a.owner_id, account: "available", amount: a.current_bid, kind: "SETTLE", auction: a.id },
    ]);
    a.settled_at = iso(now); a.version++; this.counters.settled++;
    this.#emit({ type: "auction.updated", cause: "auction.settled", auction: this.#snapshot(a) });
  }

  /* ------------------------------------------------------------ correctness checks */
  /** What "the page stays consistent under load" means, as checks anyone can read. */
  invariants(auctionId) {
    const a = this.#get(auctionId);
    const checks = [];
    const inOrder = a.bids.every((b, i) => i === 0 || b.amount > a.bids[i - 1].amount);
    checks.push({ name: "Bid history is strictly increasing", ok: inOrder, detail: `${a.bids.length} accepted bids` });
    checks.push({ name: "Bid count matches the history", ok: a.bid_count === a.bids.length, detail: `${a.bid_count} counted, ${a.bids.length} stored` });
    const last = a.bids[a.bids.length - 1];
    checks.push({ name: "Exactly one leader, and it is the last accepted bid", ok: !last || (a.current_bidder_id === last.user_id && a.current_bid === last.amount), detail: last ? `leader holds ${last.amount}` : "no bids" });
    const rec = this.reconcile();
    checks.push({ name: "Held money equals the leading bids", ok: rec.reserved_in_wallets === rec.active_reservations_total, detail: `${rec.reserved_in_wallets} held, ${rec.active_reservations_total} leading` });
    checks.push({ name: "Every ledger journal sums to zero", ok: rec.unbalanced_journals === 0 && rec.mismatches === 0, detail: `${this.ledger.length} ledger lines` });
    return { ok: checks.every(c => c.ok), checks };
  }

  /* ------------------------------------------------------------ operations */
  /** Application metrics in the same shape as GET /v1/admin/metrics. */
  metrics(extra = {}) {
    const fam = (name, type, help, samples) => ({ name, type, help, samples });
    const bounds = [0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5];
    const secs = this.counters.durations.map(ms => ms / 1000);
    const buckets = bounds.map(le => ({ le, count: secs.filter(s => s <= le).length }));
    const pending = extra.outboxPending ?? 0;
    return {
      time: iso(this.now()),
      families: [
        fam("auction_bids_total", "counter", "Bid attempts by outcome.", Object.entries(this.counters.bids).map(([result, value]) => ({ labels: { result }, value }))),
        fam("auction_bid_duration_seconds", "histogram", "Time to place a bid.", [{ labels: { strategy: "pessimistic" }, count: secs.length, sum: secs.reduce((a, b) => a + b, 0), buckets: [...buckets, { le: Infinity, count: secs.length }].map(b => ({ le: b.le === Infinity ? 1e9 : b.le, count: b.count })) }]),
        fam("auction_outbox_pending", "gauge", "Outbox events waiting to be delivered.", [{ value: pending }]),
        fam("auction_outbox_dead_lettered", "gauge", "Outbox events parked after exhausting retries.", [{ value: this.deadLetters.length }]),
        fam("auction_outbox_events_total", "counter", "Outbox events handled.", [{ labels: { type: "bid.placed", result: "processed" }, value: this.counters.bids.accepted || 0 }]),
        fam("auction_ws_connections", "gauge", "Open WebSocket connections.", [{ value: extra.connections ?? 0 }]),
        fam("auction_kafka_consumer_lag", "gauge", "Records waiting to be consumed.", [{ labels: { group: "bid-workers", topic: "auction-bids", partition: "0" }, value: extra.kafkaLag ?? 0 }]),
        fam("auction_circuit_breaker_state", "gauge", "Circuit breaker state: 0 closed, 1 half-open, 2 open.", ["elasticsearch", "bidding", "smtp"].map(name => ({ labels: { name }, value: name === "smtp" && this.deadLetters.length ? 2 : 0 }))),
        fam("auction_db_pool_acquired_connections", "gauge", "Connections currently in use.", [{ value: extra.dbBusy ?? 1 }]),
        fam("auction_db_pool_max_connections", "gauge", "Maximum size of the connection pool.", [{ value: 16 }]),
      ],
    };
  }

  failedOutbox() { return this.deadLetters.map(d => ({ ...d })); }
  retryOutbox(id) {
    const i = this.deadLetters.findIndex(d => d.id === id);
    if (i < 0) throw new SimError(404, "no dead-lettered event with that id");
    this.deadLetters.splice(i, 1);
  }

  /* ------------------------------------------------------------ persistence */
  toJSON() {
    return {
      v: 1, seq: this.seq, users: [...this.users.values()], wallets: [...this.wallets], ledger: this.ledger, auctions: [...this.auctions.values()], order: this.order, bidLog: this.bidLog.length,
      idem: [...this.idem], depositKeys: [...this.depositKeys.keys()], deadLetters: this.deadLetters, counters: this.counters, refresh: [...this.refreshTokens],
    };
  }
  static fromJSON(j, opts) {
    const w = new World(opts);
    w.seq = j.seq; j.users.forEach(u => { w.users.set(u.id, u); w.emails.set(u.email.toLowerCase(), u.id); });
    w.wallets = new Map(j.wallets); w.ledger = j.ledger; j.auctions.forEach(a => w.auctions.set(a.id, a)); w.order = j.order;
    w.idem = new Map(j.idem); j.depositKeys.forEach(k => w.depositKeys.set(k, true)); w.deadLetters = j.deadLetters; w.counters = j.counters; w.refreshTokens = new Map(j.refresh);
    for (const a of w.auctions.values()) a.bids.forEach(b => w.bidLog.push(b));
    return w;
  }
}

/* ---------- helpers ---------- */
function outcome(e) {
  if (e.body.minimum_amount != null) return "too_low";
  return ({ "insufficient funds": "insufficient_funds", "auction is not accepting bids": "not_active", "auction has ended": "ended", "sellers cannot bid on their own auction": "own_auction", "auction not found": "not_found" })[e.message] || "invalid";
}

function editsWithin1(a, b) {
  if (Math.abs(a.length - b.length) > 1) return false;
  // does any substring of `a` of b's length (+-1) sit within one edit of b?
  for (let i = 0; i <= a.length - b.length + 1; i++) {
    for (const len of [b.length - 1, b.length, b.length + 1]) {
      const s = a.slice(i, i + len);
      if (s.length && oneEdit(s, b)) return true;
    }
  }
  return false;
}
function oneEdit(s, t) {
  if (s === t) return true;
  if (Math.abs(s.length - t.length) > 1) return false;
  let i = 0; while (i < s.length && i < t.length && s[i] === t[i]) i++;
  // same length: one substitution, or two neighbours swapped (Elasticsearch counts a transposition as one edit)
  if (s.length === t.length) return s.slice(i + 1) === t.slice(i + 1) || (s[i] === t[i + 1] && s[i + 1] === t[i] && s.slice(i + 2) === t.slice(i + 2));
  return s.length > t.length ? s.slice(i + 1) === t.slice(i) : t.slice(i + 1) === s.slice(i);
}
function highlight(name, terms) {
  let out = name;
  for (const t of terms) out = out.replace(new RegExp("(" + t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + ")", "ig"), "<em>$1</em>");
  return out;
}
