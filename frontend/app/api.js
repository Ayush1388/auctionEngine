// REST client for the real API.
//
//  - sends the access token, refreshes it once when it is about to expire or
//    a request comes back 401, and retries the request
//  - turns every non-2xx answer into an ApiError that keeps the server's own
//    words: the field errors, the minimum bid, Retry-After, the request ID
//  - returns the response headers and timings with the data, because the
//    "behind the bid" panel needs them
export class ApiError extends Error {
  constructor(status, body, res) {
    super((body && body.error) || (status === 0 ? "Cannot reach the server" : `Request failed (${status})`));
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.fields = (body && body.fields) || null;
    this.minimum = body && body.minimum_amount != null ? body.minimum_amount : null;
    this.retryAfter = res ? Number(res.headers.get("retry-after")) || null : null;
    this.requestId = res ? res.headers.get("x-request-id") : null;
    this.headers = res ? res.headers : null;
  }
  get network() { return this.status === 0; }
}

export function createApi({ base, session, timeoutMs = 10000 }) {
  let refreshing = null;

  async function send(method, path, { body, headers = {}, auth = false, signal } = {}) {
    const h = { Accept: "application/json", ...headers };
    if (body !== undefined) h["Content-Type"] = "application/json";
    if (auth && session.access) h.Authorization = "Bearer " + session.access;
    const ctl = new AbortController();
    const timer = setTimeout(() => ctl.abort(), timeoutMs);
    if (signal) signal.addEventListener("abort", () => ctl.abort(), { once: true });
    const sentAt = Date.now(), t0 = performance.now();
    try {
      const res = await fetch(base + path, { method, headers: h, body: body === undefined ? undefined : JSON.stringify(body), signal: ctl.signal });
      return { res, sentAt, t0, t1: performance.now() };
    } catch (e) {
      throw new ApiError(0, { error: e.name === "AbortError" ? "The server took too long to answer" : "Cannot reach the server" }, null);
    } finally { clearTimeout(timer); }
  }

  /** Swap the refresh token for new tokens. Concurrent callers share one request. */
  function refresh() {
    if (!session.refresh) return Promise.reject(new ApiError(401, { error: "Not signed in" }, null));
    return refreshing ||= (async () => {
      try {
        const { res } = await send("POST", "/v1/auth/refresh", { body: { refresh_token: session.refresh } });
        if (!res.ok) { session.clear(); throw new ApiError(res.status, await res.json().catch(() => null), res); }
        session.set(await res.json());
      } finally { refreshing = null; }
    })();
  }

  /**
   * @returns {Promise<{data, status, headers, ms, sentAt}>}
   * `auth: true` sends the token (and refreshes it); false never does.
   */
  async function request(method, path, opts = {}) {
    if (opts.auth && session.refresh && session.expiresIn < 20000) { try { await refresh(); } catch { /* the request below reports 401 */ } }
    let out = await send(method, path, opts);
    if (out.res.status === 401 && opts.auth && session.refresh) {
      try { await refresh(); out = await send(method, path, opts); } catch { /* fall through with the 401 */ }
    }
    const { res } = out;
    const text = res.status === 204 ? "" : await res.text();
    let data = null;
    if (text) { try { data = JSON.parse(text); } catch { data = { error: text.slice(0, 200) }; } }
    if (!res.ok) throw new ApiError(res.status, data, res);
    return { data, status: res.status, headers: res.headers, ms: out.t1 - out.t0, sentAt: out.sentAt, t0: out.t0 };
  }

  return {
    base, request, refresh,
    get: (path, opts) => request("GET", path, opts),
    post: (path, body, opts) => request("POST", path, { ...opts, body }),
  };
}
