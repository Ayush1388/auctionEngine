// Small helpers shared by every page. No dependencies.

/* ---------- money ---------- */
// The backend stores integers in the smallest currency unit, so $118,000 is 11_800_000.
export const UNIT = 100;
export const usd = (cents, { decimals = false } = {}) => {
  if (cents == null || Number.isNaN(cents)) return "-";
  const v = cents / UNIT;
  const s = Math.abs(v).toLocaleString("en-US", { minimumFractionDigits: decimals ? 2 : 0, maximumFractionDigits: decimals ? 2 : 0 });
  return (v < 0 ? "-$" : "$") + s;
};
export const toCents = dollars => Math.round(dollars * UNIT);
/** Parse "$118,000" or "118000" typed by a person into cents (or 0). */
export const parseUsd = text => toCents(Number(String(text).replace(/[^0-9.]/g, "")) || 0);

/* ---------- time ---------- */
const pad = n => String(n).padStart(2, "0");
export const clock = secs => {
  secs = Math.max(0, Math.floor(secs));
  return [Math.floor(secs / 3600), Math.floor(secs % 3600 / 60), secs % 60].map(pad).join(":");
};
export const ago = (iso, now = Date.now()) => {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
  if (s < 5) return "just now";
  if (s < 60) return s + " s ago";
  if (s < 3600) return Math.floor(s / 60) + " min ago";
  if (s < 86400) return Math.floor(s / 3600) + " h ago";
  return Math.floor(s / 86400) + " d ago";
};
export const dateTime = iso => new Date(iso).toLocaleString("en-US", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
export const timeOnly = iso => new Date(iso).toLocaleTimeString("en-US", { hour12: false });
/** Milliseconds with a sensible number of digits: 0.42 ms, 8.9 ms, 142 ms, 1.2 s. */
export const dur = n => {
  if (n == null || Number.isNaN(n)) return "-";
  if (n < 1) return n.toFixed(2) + " ms";
  if (n < 100) return n.toFixed(1) + " ms";
  if (n < 1000) return Math.round(n) + " ms";
  return (n / 1000).toFixed(2) + " s";
};

/* ---------- ids ---------- */
export const uuid = () => (crypto.randomUUID ? crypto.randomUUID() : "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, c => { const r = Math.random() * 16 | 0; return (c === "x" ? r : (r & 3 | 8)).toString(16); }));
export const short = id => (id || "").slice(0, 8);

/* ---------- DOM ---------- */
export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const ESC = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
export const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ESC[c]);
class Raw { constructor(s) { this.s = s; } toString() { return this.s; } }
/** Mark a string as already-safe HTML. */
export const raw = s => new Raw(s);
/**
 * Tagged template that escapes every interpolated value unless it is raw()
 * or the result of another html`` call. Titles and descriptions come from the
 * API, so nothing from the network may reach innerHTML unescaped.
 */
export function html(strings, ...vals) {
  let out = strings[0];
  vals.forEach((v, i) => {
    const part = Array.isArray(v) ? v.map(x => (x instanceof Raw ? x.s : esc(x))).join("") : (v instanceof Raw ? v.s : esc(v));
    out += part + strings[i + 1];
  });
  return new Raw(out);
}
export const setHTML = (el, h) => { el.innerHTML = String(h); return el; };

export function debounce(fn, wait = 200) {
  let t;
  const d = (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), wait); };
  d.cancel = () => clearTimeout(t);
  return d;
}
export const sleep = ms => new Promise(r => setTimeout(r, ms));
export const clamp = (n, lo, hi) => Math.min(hi, Math.max(lo, n));

/** Safe localStorage / sessionStorage access (private windows can throw). */
const store = kind => ({
  get(k, fallback = null) { try { const v = window[kind].getItem(k); return v == null ? fallback : v; } catch { return fallback; } },
  set(k, v) { try { window[kind].setItem(k, v); return true; } catch { return false; } },
  del(k) { try { window[kind].removeItem(k); } catch { /* ignore */ } },
  json(k, fallback = null) { try { const v = window[kind].getItem(k); return v ? JSON.parse(v) : fallback; } catch { return fallback; } },
  setJson(k, v) { try { window[kind].setItem(k, JSON.stringify(v)); return true; } catch { return false; } },
});
export const local = store("localStorage");
export const tab = store("sessionStorage");

/** Minimal event emitter. */
export class Emitter {
  #h = new Map();
  on(type, fn) { (this.#h.get(type) || this.#h.set(type, new Set()).get(type)).add(fn); return () => this.off(type, fn); }
  off(type, fn) { this.#h.get(type)?.delete(fn); }
  emit(type, ...a) { this.#h.get(type)?.forEach(fn => { try { fn(...a); } catch (e) { console.error(e); } }); }
}

/** Respect the reduced-motion preference everywhere motion is added in JS. */
export const reduceMotion = () => matchMedia("(prefers-reduced-motion: reduce)").matches;
