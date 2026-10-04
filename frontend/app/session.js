// Who is signed in, for this browser tab.
//
// Tokens live in sessionStorage, not localStorage, on purpose: refresh tokens
// rotate and the server treats a reused one as theft, so two tabs sharing one
// token would sign each other out. It also lets you sign in as a different
// person in each window, which is what the two-window bidding demo needs.
import { Emitter, tab } from "./util.js";

const b64 = s => decodeURIComponent(atob(s.replace(/-/g, "+").replace(/_/g, "/")).split("").map(c => "%" + c.charCodeAt(0).toString(16).padStart(2, "0")).join(""));
/** Read a JWT's claims for display. This does not verify anything; the server does. */
export function claims(token) {
  try { return JSON.parse(b64(token.split(".")[1])); } catch { return {}; }
}

export class Session extends Emitter {
  #key;
  #cache;
  constructor(scope) { super(); this.#key = "marque-session:" + scope; this.#cache = tab.json(this.#key); }

  get() { return this.#cache; }
  get user() { return this.#cache?.user || null; }
  get id() { return this.#cache?.user?.id || null; }
  get isAdmin() { return this.#cache?.user?.role === "admin"; }
  get access() { return this.#cache?.access || null; }
  get refresh() { return this.#cache?.refresh || null; }
  /** Milliseconds until the access token expires (negative once expired). */
  get expiresIn() { return this.#cache?.exp ? this.#cache.exp * 1000 - Date.now() : -1; }

  /** Store a login or refresh response. `user` is kept from the previous value when absent. */
  set(tokens, user) {
    const c = claims(tokens.access_token);
    const prev = this.#cache?.user;
    const u = user ? { id: user.id || c.user_id, email: user.email, role: c.role || "user" } : (prev ? { ...prev, role: c.role || prev.role } : { id: c.user_id, email: "", role: c.role || "user" });
    this.#cache = { access: tokens.access_token, refresh: tokens.refresh_token, exp: c.exp, user: u };
    tab.setJson(this.#key, this.#cache);
    this.emit("change", this.user);
  }
  clear() {
    if (!this.#cache) return;
    this.#cache = null; tab.del(this.#key);
    this.emit("change", null);
  }
}
