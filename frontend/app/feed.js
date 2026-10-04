// Live updates over the API's WebSocket (GET /v1/ws).
//
//   feed.subscribe(auctionId, (msg, meta) => ...)  ->  unsubscribe()
//
// The server sends a full snapshot with a version on every change. Messages can
// arrive out of order (several outbox workers, retries), so a snapshot is only
// passed on when its version is higher than the last one seen for that auction.
// After a reconnect the feed emits "resync" so pages reload what they show:
// pub/sub is fire-and-forget, a dropped connection can miss updates.
import { Emitter } from "./util.js";

export class Feed extends Emitter {
  #url; #ws = null; #subs = new Map(); #versions = new Map(); #backoff = 500; #timer = null; #wasOpen = false; #closed = false;
  state = "idle";
  /** serverTime - clientTime in ms, estimated from message timestamps (lowest sample wins). */
  clockOffset = null;

  constructor(url) { super(); this.#url = url; }

  start() { if (!this.#ws && !this.#closed) this.#connect(); return this; }
  stop() { this.#closed = true; clearTimeout(this.#timer); this.#ws?.close(1000); this.#set("closed"); }

  /** Server clock now, as a Date.now()-style number. */
  serverNow() { return Date.now() + (this.clockOffset || 0); }

  subscribe(auctionId, fn) {
    this.start();
    let set = this.#subs.get(auctionId);
    if (!set) { set = new Set(); this.#subs.set(auctionId, set); this.#send({ action: "subscribe", auction_id: auctionId }); }
    set.add(fn);
    return () => {
      set.delete(fn);
      if (!set.size) { this.#subs.delete(auctionId); this.#versions.delete(auctionId); this.#send({ action: "unsubscribe", auction_id: auctionId }); }
    };
  }

  #set(state, extra) { this.state = state; this.emit("state", { state, ...extra }); }
  #send(obj) { if (this.#ws && this.#ws.readyState === 1) this.#ws.send(JSON.stringify(obj)); }

  #connect() {
    this.#set("connecting");
    let ws;
    try { ws = new WebSocket(this.#url); } catch { this.#retry(); return; }
    this.#ws = ws;
    ws.onopen = () => {
      this.#backoff = 500;
      this.#set("open");
      for (const id of this.#subs.keys()) this.#send({ action: "subscribe", auction_id: id });
      if (this.#wasOpen) this.emit("resync");
      this.#wasOpen = true;
    };
    ws.onmessage = e => {
      const receivedAt = performance.now(), wallAt = Date.now();
      let m; try { m = JSON.parse(e.data); } catch { return; }
      if (m.type !== "auction.updated" || !m.auction) return;
      const id = m.auction.id, v = m.auction.version;
      if (typeof v === "number") {
        if (v <= (this.#versions.get(id) ?? -1)) return;      // stale or duplicate
        this.#versions.set(id, v);
      }
      if (m.timing && m.timing.sent_at) {
        const sample = new Date(m.timing.sent_at).getTime() - wallAt;
        this.clockOffset = this.clockOffset === null ? sample : Math.min(this.clockOffset, sample);
      }
      const meta = { receivedAt, wallAt };
      this.#subs.get(id)?.forEach(fn => { try { fn(m, meta); } catch (err) { console.error(err); } });
      this.emit("update", m, meta);
    };
    ws.onclose = () => { this.#ws = null; if (!this.#closed) this.#retry(); };
    ws.onerror = () => { /* onclose follows */ };
  }

  #retry() {
    const wait = this.#backoff;
    this.#backoff = Math.min(this.#backoff * 2, 10000);
    this.#set("retrying", { retryIn: wait });
    this.#timer = setTimeout(() => this.#connect(), wait);
  }
}
