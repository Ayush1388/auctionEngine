// Backend adapter for the real API. The demo engine (sim.js) implements the same
// surface, so pages never know which one they are talking to.
import { createApi, ApiError } from "./api.js";
import { Feed } from "./feed.js";
import { Session } from "./session.js";
import { wsUrl, config } from "./config.js";
import { uuid } from "./util.js";

/** "lock;dur=0.31, commit;dur=2.1" -> { lock: 0.31, commit: 2.1 } (milliseconds) */
export function parseServerTiming(h) {
  if (!h) return null;
  const out = {};
  for (const part of h.split(",")) {
    const [name, ...params] = part.trim().split(";");
    const d = params.map(p => p.trim()).find(p => p.startsWith("dur="));
    if (name && d) out[name.trim()] = Number(d.slice(4));
  }
  return Object.keys(out).length ? out : null;
}

/** RateLimit-Limit / RateLimit-Remaining -> { limit, remaining } (null when the API sends none) */
const rateOf = h => {
  const limit = Number(h.get("ratelimit-limit")), remaining = Number(h.get("ratelimit-remaining"));
  return h.get("ratelimit-limit") == null ? null : { limit, remaining };
};

const qs = o => {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(o || {})) if (v !== undefined && v !== null && v !== "") p.set(k, v);
  const s = p.toString();
  return s ? "?" + s : "";
};

export function createLive(base = config.api) {
  const session = new Session(base);
  const api = createApi({ base, session });
  const feed = new Feed(wsUrl(base));
  let caps = { kafka: false };

  const be = {
    kind: "live",
    label: "Live backend",
    detail: base.replace(/^https?:\/\//, ""),
    base, session, api, feed,
    caps: { stress: true, async: false, serverTiming: true },

    async ready() {
      const r = await api.get("/readyz");
      caps = { kafka: "kafka" in (r.data?.checks || {}) };
      be.caps.async = caps.kafka;
      return r.data;
    },

    /* ---------- auth ---------- */
    me: () => session.user,
    onAuth: fn => session.on("change", fn),
    async register(email, password) {
      const { data } = await api.post("/v1/users/register", { email, password });
      return { user: data, activation: null };
    },
    async activate(token) { return (await api.get("/v1/users/activate" + qs({ token }))).data; },
    async resendActivation(email) { await api.post("/v1/users/resend-activation", { email }); },
    async login(email, password) {
      const { data } = await api.post("/v1/users/login", { email, password });
      session.set(data, data.user);
      return session.user;
    },
    async logout() {
      const refresh = session.refresh;
      session.clear();
      if (refresh) { try { await api.post("/v1/auth/logout", { refresh_token: refresh }); } catch { /* already signed out locally */ } }
    },

    /* ---------- auctions ---------- */
    async listAuctions(o = {}) {
      const { data } = await api.get("/v1/auctions" + qs({ status: o.status, owner: o.owner, limit: o.limit, cursor: o.cursor }), { auth: !!o.owner });
      return data;
    },
    /** Every auction matching a filter, following the cursor (bounded). */
    async allAuctions(o = {}, maxPages = 5) {
      const out = []; let cursor;
      for (let i = 0; i < maxPages; i++) {
        const page = await be.listAuctions({ ...o, limit: 100, cursor });
        out.push(...page.auctions);
        if (!page.next_cursor) break;
        cursor = page.next_cursor;
      }
      return out;
    },
    async getAuction(id) { return (await api.get("/v1/auctions/" + id)).data; },
    async trending(limit = 8) { return (await api.get("/v1/auctions/trending" + qs({ limit }))).data.auctions; },
    async search(q, o = {}) { return (await api.get("/v1/auctions/search" + qs({ q, status: o.status, limit: o.limit, cursor: o.cursor }))).data; },
    async suggest(q, limit = 6) { return (await api.get("/v1/auctions/suggest" + qs({ q, limit }))).data.suggestions; },
    async createAuction(input) { return (await api.post("/v1/auctions", input, { auth: true })).data; },
    async cancelAuction(id) { return (await api.post(`/v1/auctions/${id}/cancel`, undefined, { auth: true })).data; },
    async bids(auctionId, o = {}) { return (await api.get(`/v1/auctions/${auctionId}/bids` + qs({ limit: o.limit, cursor: o.cursor }))).data; },

    /**
     * Place a bid. Never throws for a rejected bid: the caller gets the outcome
     * and the timing evidence either way.
     */
    async placeBid(auctionId, amount, { key = uuid(), queue = false } = {}) {
      const headers = { "Idempotency-Key": key };
      if (queue && caps.kafka) headers.Prefer = "respond-async";
      const base = { key, amount, auctionId };
      const tSent = Date.now(), tStart = performance.now();
      try {
        const r = await api.request("POST", `/v1/auctions/${auctionId}/bids`, { body: { amount }, headers, auth: true });
        return { ...base, ok: true, status: r.status, data: r.data, replayed: r.status === 200 || !!r.data?.replayed, queued: r.status === 202,
          requestId: r.headers.get("x-request-id"), traceId: r.headers.get("x-trace-id"), serverTiming: parseServerTiming(r.headers.get("server-timing")), rate: rateOf(r.headers),
          sentAt: r.sentAt, ms: r.ms, t0: r.t0 };
      } catch (e) {
        if (!(e instanceof ApiError)) throw e;
        return { ...base, ok: false, status: e.status, error: e, data: e.body, replayed: false, requestId: e.requestId, traceId: e.headers?.get("x-trace-id") || null,
          serverTiming: parseServerTiming(e.headers?.get("server-timing")), rate: e.headers ? rateOf(e.headers) : null, sentAt: tSent, ms: performance.now() - tStart, t0: tStart };
      }
    },
    /* maximum (proxy) bids */
    async setProxy(auctionId, max) { return (await api.request("PUT", `/v1/auctions/${auctionId}/proxy-bid`, { body: { max_amount: max }, auth: true })).data; },
    async getProxy(auctionId) {
      try { return (await api.get(`/v1/auctions/${auctionId}/proxy-bid`, { auth: true })).data; }
      catch (e) { if (e instanceof ApiError && e.status === 404) return null; throw e; }
    },
    async cancelProxy(auctionId) { await api.request("DELETE", `/v1/auctions/${auctionId}/proxy-bid`, { auth: true }); },

    /* the operator's fault switchboard: /v1/admin/chaos (needs CHAOS_ENABLED=true on the API) */
    chaos: {
      get: async () => (await api.get("/v1/admin/chaos", { auth: true })).data,
      set: async (fault, active, o = {}) => (await api.request("PUT", "/v1/admin/chaos", { body: { fault, active, ...(o.slowMs != null ? { slow_query_ms: o.slowMs } : {}) }, auth: true })).data,
      reset: async () => (await api.post("/v1/admin/chaos/reset", undefined, { auth: true })).data,
      setRateLimits: async () => { throw new ApiError(400, { error: "Rate limits on the real API are set with RATE_LIMITS=off" }, null); },
    },
    async bidRequest(id) { return (await api.get("/v1/bid-requests/" + id, { auth: true })).data; },

    /* ---------- wallet ---------- */
    async wallet() { return (await api.get("/v1/wallet", { auth: true })).data; },
    async deposit(amount, key = uuid()) { return (await api.post("/v1/wallet/deposits", { amount }, { auth: true, headers: { "Idempotency-Key": key } })).data; },
    async ledger(o = {}) { return (await api.get("/v1/wallet/ledger" + qs({ limit: o.limit, before: o.before }), { auth: true })).data; },

    /* ---------- operations ---------- */
    async readyz() {
      try { return (await api.get("/readyz")).data; }
      catch (e) { if (e instanceof ApiError && e.body && e.body.status) return e.body; throw e; }
    },
    async livez() { const t = performance.now(); await api.get("/livez"); return { ok: true, ms: performance.now() - t }; },
    async metrics() { return (await api.get("/v1/admin/metrics", { auth: true })).data; },
    async reconcile() {
      try { return (await api.get("/v1/admin/reconcile", { auth: true })).data; }
      catch (e) { if (e instanceof ApiError && e.status === 409 && e.body) return e.body; throw e; }
    },
    async failedOutbox() { return (await api.get("/v1/admin/outbox/failed", { auth: true })).data.events; },
    async retryOutbox(id) { await api.post(`/v1/admin/outbox/${id}/retry`, undefined, { auth: true }); },

    /* ---------- demo bots: built into the API (DEMO_BOTS_ENABLED=true), switched on and off here ---------- */
    bots: {
      /** { enabled, ambient: { on, bids, refused, ... }, message } */
      async get() { return (await api.get("/v1/demo/bots")).data; },
      async setAmbient(on) { return (await api.request("PUT", "/v1/demo/bots", { body: { ambient: !!on }, auth: true })).data; },
    },

    stress: {
      async available() {
        try { return !!(await api.get("/v1/demo/bots")).data.enabled; } catch { return false; }
      },
      /** Starts a run; emits {type:"progress"|"done"|"error"} to `onEvent`. Returns { stop }. */
      async start({ auctionId, bidders, rounds }, onEvent) {
        const { data } = await api.post("/v1/demo/stress", { auction_id: auctionId, bidders, rounds }, { auth: true });
        const ctl = new AbortController();
        (async () => {
          try {
            if (session.refresh && session.expiresIn < 20000) await api.refresh().catch(() => {});
            const r = await fetch(base + "/v1/demo/stress/" + data.id + "/events", { headers: { Authorization: "Bearer " + session.access }, signal: ctl.signal });
            if (!r.ok || !r.body) throw new Error("The progress stream answered " + r.status);
            const reader = r.body.getReader(), dec = new TextDecoder();
            let buf = "";
            for (;;) {
              const { value, done } = await reader.read();
              if (done) break;
              buf += dec.decode(value, { stream: true });
              let k;
              while ((k = buf.indexOf("\n\n")) >= 0) {
                const line = buf.slice(0, k).replace(/^data: /, ""); buf = buf.slice(k + 2);
                if (!line.trim()) continue;
                const ev = JSON.parse(line);
                onEvent(ev);
                if (ev.type === "done" || ev.type === "error") { ctl.abort(); return; }
              }
            }
          } catch (e) { if (e.name !== "AbortError") onEvent({ type: "error", error: e.message || "Lost the progress stream" }); }
        })();
        return { stop: () => ctl.abort() };
      },
    },
  };
  return be;
}
