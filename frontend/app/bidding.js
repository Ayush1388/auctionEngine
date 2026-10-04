// Everything about ONE lot that is not drawing: loading it, following it live,
// placing bids and explaining what happened to each one.
//
// Events (all emit plain data):
//   lot        the lot changed (a live update or a reload)
//   history    the bid list changed
//   wallet     my balances changed
//   attempt    one of my bid attempts was created or finished
//   outcome    what a bid attempt meant for the bidder: { kind, message, ... }
//   outbid     someone took the lead from me
//   extended   anti-sniping moved the close
//   event      a raw live update, for the event log
import { Emitter, uuid, usd, short, dur } from "./util.js";
import { toLot, applySnapshot } from "./model.js";

/** People are not named by the API, so a bidder is a stable number made from their ID. */
export function bidderLabel(id, meId) {
  if (!id) return "No bids yet";
  if (id === meId) return "You";
  let h = 0; for (const c of id) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return "Bidder #" + (100 + h % 900);
}

const DUPLICATE_WINDOW_MS = 4000;

export class LotSession extends Emitter {
  constructor(be, id) {
    super();
    this.be = be; this.id = id;
    this.lot = null; this.history = []; this.wallet = null; this.attempts = []; this.log = [];
    this.n = 0; this.intent = null; this.unsub = null; this.recentWs = new Map(); this.lastOther = 0; this.stale = 0;
    this.offResync = null;
  }

  get me() { return this.be.session.id; }
  get standing() { return !this.lot || !this.me ? null : (this.lot.bidderId === this.me ? "leading" : (this.history.some(b => b.user_id === this.me) ? "outbid" : "watching")); }

  async load() {
    const [a, h] = await Promise.all([this.be.getAuction(this.id), this.be.bids(this.id, { limit: 50 })]);
    this.lot = toLot(a); this.history = h.bids;
    await this.loadWallet();
    this.emit("lot", this.lot); this.emit("history", this.history);
    if (!this.unsub) {
      this.unsub = this.be.feed.subscribe(this.id, (ev, meta) => this.#onUpdate(ev, meta));
      this.offResync = this.be.feed.on("resync", () => this.reload());
    }
    return this.lot;
  }

  async reload() {
    try {
      const [a, h] = await Promise.all([this.be.getAuction(this.id), this.be.bids(this.id, { limit: 50 })]);
      this.lot = { ...toLot(a), version: this.lot?.version ?? null }; this.history = h.bids;
      this.emit("lot", this.lot); this.emit("history", this.history);
      this.loadWallet();
    } catch { /* the next update or resync tries again */ }
  }

  async loadWallet() {
    if (!this.me) { this.wallet = null; return null; }
    try { this.wallet = await this.be.wallet(); this.emit("wallet", this.wallet); } catch { /* signed out meanwhile */ }
    return this.wallet;
  }

  destroy() { this.unsub?.(); this.offResync?.(); this.unsub = null; }

  /* ------------------------------------------------------------ live updates */
  #onUpdate(ev, meta) {
    const s = ev.auction, prev = this.lot;
    if (!prev) return;
    const entry = { at: Date.now(), version: s.version, cause: ev.cause, amount: s.current_bid, bid: ev.bid || null, timing: ev.timing || null, meta, mine: !!(ev.bid && ev.bid.bidder_id === this.me) };
    this.log.unshift(entry); if (this.log.length > 40) this.log.pop();
    if (ev.bid) { this.recentWs.set(ev.bid.id, { ev, meta }); if (this.recentWs.size > 30) this.recentWs.delete(this.recentWs.keys().next().value); }

    this.lot = applySnapshot(prev, s);
    const extended = Date.parse(s.ends_at) > Date.parse(prev.endsAt) && s.extensions > prev.extensions;

    if (ev.bid) {
      this.history = [{ id: ev.bid.id, auction_id: s.id, user_id: ev.bid.bidder_id, amount: ev.bid.amount, created_at: ev.timing?.placed_at || new Date().toISOString() }, ...this.history.filter(b => b.id !== ev.bid.id)].slice(0, 50);
      if (ev.bid.bidder_id !== this.me) this.lastOther = performance.now();
      if (ev.bid.previous_bidder_id === this.me && ev.bid.bidder_id !== this.me) {
        this.loadWallet();
        this.emit("outbid", { by: ev.bid.bidder_id, amount: ev.bid.amount, hold: prev.current });
      }
      const at = this.attempts.find(a => a.bidId === ev.bid.id);
      if (at) this.#attachWs(at, ev, meta);
      this.emit("history", this.history);
    } else if (["auction.completed", "auction.settled", "auction.cancelled", "auction.activated", "demo.fast_forward"].includes(ev.cause)) {
      this.reload(); if (ev.cause === "auction.completed" || ev.cause === "auction.settled") this.loadWallet();
    }
    if (extended) this.emit("extended", { from: prev.endsAt, to: s.ends_at, extensions: s.extensions });
    this.emit("lot", this.lot);
    this.emit("event", entry);
  }

  #attachWs(at, ev, meta) {
    at.ws = { ev, receivedAt: meta.receivedAt, e2e: meta.receivedAt - at.t0, outbox: ev.timing ? Date.parse(ev.timing.sent_at) - Date.parse(ev.timing.placed_at) : null };
    this.emit("attempt", at);
  }

  /* ------------------------------------------------------------ placing a bid */
  /**
   * One click = one intent. A second click on the same amount within a few
   * seconds reuses the first click's Idempotency-Key, so the server answers
   * "already placed" instead of bidding twice. A different amount is a new bid.
   */
  async place(amount, { queue = false } = {}) {
    if (!this.me) { const o = { kind: "auth", message: "Sign in to bid." }; this.emit("outcome", o); return { outcome: o }; }
    const now = Date.now();
    if (!this.intent || this.intent.amount !== amount || now - this.intent.at > DUPLICATE_WINDOW_MS) this.intent = { amount, key: uuid(), at: now };
    else this.intent.at = now;
    const key = this.intent.key, lotAtClick = this.lot;

    const at = { n: ++this.n, id: uuid(), auctionId: this.id, amount, key, startedAt: now, t0: performance.now(), state: "sent", minAtClick: lotAtClick.minNext, queue };
    this.attempts.unshift(at); if (this.attempts.length > 25) this.attempts.pop();
    this.emit("attempt", at);

    const r = await this.be.placeBid(this.id, amount, { key, queue });
    Object.assign(at, { state: "done", ok: r.ok, status: r.status, replayed: r.replayed, queued: r.queued, requestId: r.requestId, traceId: r.traceId, rtt: r.ms, timing: r.serverTiming, sentAt: r.sentAt, t0: r.t0, error: r.error || null, data: r.data });
    if (r.ok && r.data?.bid) {
      at.bidId = r.data.bid.id;
      const w = this.recentWs.get(at.bidId); if (w) this.#attachWs(at, w.ev, w.meta);
    }
    let outcome = this.#classify(at, r, lotAtClick);
    // Two clicks on one amount are one intent: whichever answer arrives last, say what really happened.
    const twins = this.attempts.filter(a => a.key === key && a.state === "done" && a.ok);
    if (twins.length > 1 && twins.some(a => !a.replayed)) {
      outcome = { kind: "leading", duplicate: true, amount: at.amount, message: `You lead at ${usd(at.amount)}. You clicked twice, but both clicks carried the same Idempotency-Key, so the server placed one bid and recognised the other as a repeat.` };
      twins.forEach(a => { a.outcome = outcome; });
    }
    at.outcome = outcome;
    this.emit("attempt", at);
    this.emit("outcome", outcome);
    if (r.ok) { if (!r.replayed) { this.#applyOwnBid(r.data); } this.loadWallet(); }
    else if (outcome.kind === "collision" || outcome.kind === "toolow") this.reload();
    return { attempt: at, outcome };
  }

  /** The bid response arrives with the new price; show it at once, the WebSocket confirms moments later. */
  #applyOwnBid(d) {
    if (!d || !this.lot) return;
    if (this.lot.bids >= d.bid_count && this.lot.current != null && this.lot.current >= d.current_bid) return;     // the live update got here first
    this.lot = { ...this.lot, current: d.current_bid, bidderId: this.me, bids: d.bid_count, minNext: d.current_bid + this.lot.step, endsAt: d.ends_at };
    this.emit("lot", this.lot);
  }

  #classify(at, r, lotAtClick) {
    const me = this.me;
    if (r.ok && r.queued) return { kind: "queued", message: "Queued. The bid goes through Kafka and is applied in order; this page updates when it lands." };
    if (r.ok && r.replayed) return { kind: "duplicate", message: "That click was already placed. The Idempotency-Key matched, so the server returned the first bid and did not bid again.", amount: at.amount };
    if (r.ok) return { kind: "leading", message: `You lead at ${usd(at.amount)}. That amount is held from your balance until you are outbid or the sale closes.`, amount: at.amount, extended: !!r.data?.extended };
    const e = r.error, body = e?.body || {};
    const lock = r.serverTiming?.lock;
    if (e?.status === 422 && body.minimum_amount != null) {
      const raced = performance.now() - this.lastOther < (r.ms ?? 0) + 250 || (lock != null && lock > 0.5) || body.minimum_amount > lotAtClick.minNext;
      return raced
        ? { kind: "collision", message: `Another bid landed first${lock != null && lock > 0.05 ? `: yours waited ${dur(lock)} behind it, then the minimum moved` : ""}. The minimum is now ${usd(body.minimum_amount)}.`, minimum: body.minimum_amount, lockMs: lock }
        : { kind: "toolow", message: `The minimum bid is ${usd(body.minimum_amount)}.`, minimum: body.minimum_amount };
    }
    if (e?.status === 422 && /insufficient/i.test(e.message)) {
      const avail = this.wallet?.available ?? 0;
      return { kind: "funds", message: `Not enough available funds. You have ${usd(avail)} and this bid needs ${usd(at.amount - (this.lot.bidderId === me ? this.lot.current : 0))}.`, short: at.amount - avail };
    }
    if (e?.status === 422) return { kind: "error", message: e.message };
    if (e?.status === 409) return { kind: "ended", message: e.message.charAt(0).toUpperCase() + e.message.slice(1) + "." };
    if (e?.status === 403) return { kind: "own", message: "You cannot bid on your own listing." };
    if (e?.status === 401) return { kind: "auth", message: "Your session ended. Sign in again to bid." };
    if (e?.status === 429) return { kind: "ratelimit", message: `Too many bids too quickly. Try again${e.retryAfter ? ` in ${e.retryAfter} s` : " in a moment"}.` };
    if (e?.status === 503) return { kind: "unavailable", message: "Bidding is busy or unavailable. Nothing was placed; try again." };
    if (e?.status === 0) return { kind: "offline", message: "Cannot reach the server. Nothing was placed." };
    return { kind: "error", message: e?.message || "Something went wrong. Nothing was placed." };
  }

  /* ------------------------------------------------------------ the held amount for this lot */
  get held() { return this.lot && this.me && this.lot.bidderId === this.me && this.lot.status === "ACTIVE" ? this.lot.current : 0; }
}

export { short };
