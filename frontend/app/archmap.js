// The architecture map: the services a bid touches, drawn as a diagram whose
// pulses are real events. A pulse is drawn when something actually happened
// (a bid committed, a search was answered, an event arrived over the WebSocket),
// never on a timer, and each node shows what the browser can honestly know
// about it: its health from the probes, and a latency only where one is measured.
import { esc, reduceMotion } from "./util.js";

const NS = "http://www.w3.org/2000/svg";
const W = 960, H = 360, NW = 156, NH = 74;

export const NODES = [
  { id: "browser", label: "Browser", sub: "this page", x: 8, y: 144 },
  { id: "gateway", label: "API gateway", sub: "auth and limits", x: 196, y: 144 },
  { id: "bidding", label: "Bidding service", sub: "decides every bid", x: 400, y: 20 },
  { id: "postgres", label: "PostgreSQL", sub: "source of truth", x: 604, y: 20 },
  { id: "outbox", label: "Outbox worker", sub: "events leave here", x: 604, y: 144 },
  { id: "kafka", label: "Kafka", sub: "event streams", x: 804, y: 20 },
  { id: "redis", label: "Redis", sub: "cache and fan-out", x: 804, y: 144 },
  { id: "elasticsearch", label: "Elasticsearch", sub: "search", x: 400, y: 268 },
  { id: "websocket", label: "WebSocket hub", sub: "live updates", x: 196, y: 268 },
];

export const EDGES = [
  ["browser", "gateway"], ["gateway", "bidding"], ["bidding", "postgres"], ["postgres", "outbox"],
  ["outbox", "kafka"], ["outbox", "redis"], ["redis", "websocket"], ["websocket", "browser"],
  ["gateway", "elasticsearch"], ["outbox", "elasticsearch"],
];

/** The paths a real action takes. */
export const PATHS = {
  bid: ["browser", "gateway", "bidding", "postgres"],
  fanout: ["postgres", "outbox", "redis", "websocket", "browser"],
  stream: ["outbox", "kafka"],
  search: ["browser", "gateway", "elasticsearch"],
  fallback: ["browser", "gateway", "postgres"],
};

const byId = Object.fromEntries(NODES.map(n => [n.id, n]));
const center = id => ({ x: byId[id].x + NW / 2, y: byId[id].y + NH / 2 });
const edgeKey = (a, b) => [a, b].sort().join("|");
const el = (tag, attrs = {}, parent) => { const n = document.createElementNS(NS, tag); for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v); parent?.append(n); return n; };

/** Where the line between two nodes leaves each box, so lines meet edges, not centres. */
function ends(a, b) {
  const A = center(a), B = center(b), dx = B.x - A.x, dy = B.y - A.y;
  const clip = (c, ux, uy) => { const t = Math.min(ux ? (NW / 2 + 4) / Math.abs(ux) : Infinity, uy ? (NH / 2 + 4) / Math.abs(uy) : Infinity); return { x: c.x + ux * t, y: c.y + uy * t }; };
  return [clip(A, dx, dy), clip(B, -dx, -dy)];
}

export function createMap(root, { onSelect } = {}) {
  root.textContent = "";
  const svg = el("svg", { viewBox: `0 0 ${W} ${H}`, class: "amap", role: "group", "aria-label": "Architecture map. Each box is a service; select one to see its health." }, root);
  const edgeEls = new Map(), nodeEls = new Map(), dotLayer = el("g", { class: "amap-dots" });
  const stats = new Map(), health = new Map();
  let selected = null, raf = 0;
  const dots = [];

  for (const [a, b] of EDGES) {
    const [p, q] = ends(a, b);
    const line = el("line", { x1: p.x, y1: p.y, x2: q.x, y2: q.y, class: "amap-edge" }, svg);
    edgeEls.set(edgeKey(a, b), line);
  }
  svg.append(dotLayer);
  for (const n of NODES) {
    const g = el("g", { class: "amap-node", tabindex: "0", role: "button", "data-id": n.id, "aria-label": `${n.label}: ${n.sub}`, transform: `translate(${n.x} ${n.y})` }, svg);
    el("rect", { width: NW, height: NH, rx: 12 }, g);
    const t1 = el("text", { x: 14, y: 26, class: "amap-title" }, g); t1.textContent = n.label;
    const t2 = el("text", { x: 14, y: 45, class: "amap-sub" }, g); t2.textContent = n.sub;
    const badge = el("text", { x: 14, y: 63, class: "amap-badge" }, g);
    nodeEls.set(n.id, { g, badge });
    const pick = () => { select(n.id); onSelect?.(n.id); };
    g.addEventListener("click", pick);
    g.addEventListener("keydown", e => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(); } });
  }

  function select(id) {
    selected = id;
    for (const [k, v] of nodeEls) v.g.classList.toggle("is-selected", k === id);
  }

  function setHealth(map) {
    for (const [id, st] of Object.entries(map)) {
      health.set(id, st);
      const n = nodeEls.get(id); if (!n) continue;
      n.g.dataset.state = st;
      n.badge.textContent = ({ ok: "ok", warn: "degraded", bad: "down", off: "off" })[st] || "";
    }
    // an edge into or out of a failing node is drawn as broken
    for (const [a, b] of EDGES) {
      const bad = health.get(a) === "bad" || health.get(b) === "bad";
      edgeEls.get(edgeKey(a, b))?.classList.toggle("is-broken", bad);
    }
  }

  function setStats(id, s) { stats.set(id, s); }

  /** Light the boxes (and the lines between them) that one stage of the tour is about. */
  function highlight(ids = []) {
    const lit = new Set(ids);
    for (const [k, v] of nodeEls) v.g.classList.toggle("is-lit", lit.has(k));
    for (const [a, b] of EDGES) edgeEls.get(edgeKey(a, b))?.classList.toggle("is-lit", lit.has(a) && lit.has(b));
    svg.classList.toggle("has-lit", lit.size > 0);
  }

  /** One pulse along `path` (node ids). `hop` is the milliseconds each hop takes on screen. */
  function pulse(path, { hop = 140, tone = "ok" } = {}) {
    if (reduceMotion()) {
      for (let i = 0; i < path.length - 1; i++) flash(path[i], path[i + 1], i * 60);
      return;
    }
    const segs = [];
    for (let i = 0; i < path.length - 1; i++) {
      if (!edgeEls.has(edgeKey(path[i], path[i + 1]))) continue;
      const [p, q] = ends(path[i], path[i + 1]);
      segs.push({ p, q, edge: edgeEls.get(edgeKey(path[i], path[i + 1])) });
    }
    if (!segs.length) return;
    const dot = el("circle", { r: 5, class: `amap-dot tone-${tone}` }, dotLayer);
    dots.push({ dot, segs, t0: performance.now(), hop });
    if (!raf) raf = requestAnimationFrame(step);
  }
  function flash(a, b, delay) {
    const e = edgeEls.get(edgeKey(a, b)); if (!e) return;
    setTimeout(() => { e.classList.add("is-hot"); setTimeout(() => e.classList.remove("is-hot"), 380); }, delay);
  }
  function step() {
    raf = 0;
    const now = performance.now();   // not the rAF timestamp: it can be earlier than a t0 taken a moment ago
    for (let i = dots.length - 1; i >= 0; i--) {
      const d = dots[i], k = Math.max(0, (now - d.t0) / d.hop), idx = Math.floor(k);
      if (idx >= d.segs.length) { d.dot.remove(); dots.splice(i, 1); continue; }
      const s = d.segs[idx], f = k - idx;
      d.dot.setAttribute("cx", s.p.x + (s.q.x - s.p.x) * f);
      d.dot.setAttribute("cy", s.p.y + (s.q.y - s.p.y) * f);
      s.edge.classList.add("is-hot");
      if (f > 0.9) setTimeout(() => s.edge.classList.remove("is-hot"), 250);
    }
    if (dots.length) raf = requestAnimationFrame(step);
  }

  return {
    select, setHealth, setStats, pulse, highlight,
    get selected() { return selected; },
    stats: id => stats.get(id),
    healthOf: id => health.get(id),
    destroy() { cancelAnimationFrame(raf); root.textContent = ""; },
  };
}

/** A line of text for the details panel. */
export const nodeFacts = {
  browser: "Your window. It places bids, holds a WebSocket open and measures every round trip with its own clock.",
  gateway: "Authenticates, applies the rate limit, validates the request and hands the bid to the bidding service. A shut-down instance drains first so a deploy sheds no requests.",
  bidding: "Locks the auction row, applies the rules on data nobody else can change, writes the bid, the wallet hold, the ledger journals and the outbox event in one transaction.",
  postgres: "The only critical dependency and the source of truth for every bid and every unit of money. Nothing that decides money reads from anywhere else.",
  outbox: "Reads events committed with a bid and delivers them at least once: to Kafka, Redis fan-out, the search index and the cache. A failing target is retried with backoff, then parked.",
  kafka: "Carries the event stream and the optional asynchronous bid queue, partitioned by auction so one lot keeps its order. A synchronous bid never touches it.",
  redis: "Cache, trending scores, shared rate-limit buckets and pub/sub between API instances. Cheap to lose: every use falls back to PostgreSQL or fails open.",
  elasticsearch: "Typo-tolerant search and type-ahead. Behind a circuit breaker; when it opens, search is answered from PostgreSQL full-text search.",
  websocket: "Holds every open browser connection and pushes each auction snapshot, with its version, so a screen never goes backwards.",
};
export { esc as _esc };
