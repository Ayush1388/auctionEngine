// The architecture map's data: the services a bid touches, the lines between them, and the paths
// that real actions take. Geometry lives here so the component only draws.
export const W = 960, H = 360, NW = 156, NH = 74;

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

export const NODE_LABEL = { browser: "Browser", gateway: "API gateway", bidding: "Bidding service", postgres: "PostgreSQL", outbox: "Outbox worker", kafka: "Kafka", redis: "Redis", elasticsearch: "Elasticsearch", websocket: "WebSocket hub" };

const byId = Object.fromEntries(NODES.map(n => [n.id, n]));
const center = id => ({ x: byId[id].x + NW / 2, y: byId[id].y + NH / 2 });
export const edgeKey = (a, b) => [a, b].sort().join("|");

/** Where the line between two nodes leaves each box, so lines meet edges, not centres. */
export function ends(a, b) {
  const A = center(a), B = center(b), dx = B.x - A.x, dy = B.y - A.y;
  const clip = (c, ux, uy) => { const t = Math.min(ux ? (NW / 2 + 4) / Math.abs(ux) : Infinity, uy ? (NH / 2 + 4) / Math.abs(uy) : Infinity); return { x: c.x + ux * t, y: c.y + uy * t }; };
  return [clip(A, dx, dy), clip(B, -dx, -dy)];
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
