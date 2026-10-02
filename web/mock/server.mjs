// Mock of the auctionEngine API for frontend development without the Go
// backend. It serves only what the Discover page reads, in the shapes of
// api/openapi.json: GET /v1/auctions, /v1/auctions/trending,
// /v1/auctions/{id} and the WebSocket at /v1/ws. It also plays the lifecycle
// worker (activates and completes auctions on time) and places a simulated
// bid every few seconds so live updates can be seen.
//
//   npm run mock                       start on :4000
//   MOCK_QUIET=1 npm run mock          no simulated bids
//   MOCK_EMPTY=1 npm run mock          no auctions at all
//   MOCK_NO_LIVE=1 npm run mock        no live auctions (only scheduled and sold)
//   MOCK_FAIL=trending,list npm run mock   those endpoints answer 500
//   MOCK_DELAY=1500 npm run mock       delay every response (ms)
import { createServer } from "node:http";
import { randomUUID } from "node:crypto";
import { WebSocketServer } from "ws";
import { SEED } from "./lots.mjs";

const PORT = Number(process.env.PORT ?? 4000);
const ORIGINS = (process.env.CORS_ALLOWED_ORIGINS ?? "http://localhost:5173,http://127.0.0.1:5173").split(",");
const FAIL = new Set((process.env.MOCK_FAIL ?? "").split(",").filter(Boolean));
const DELAY = Number(process.env.MOCK_DELAY ?? 0);
const SNIPE_WINDOW = 2 * 60_000;
const MAX_EXTENSIONS = 10;

const startedAt = Date.now();
const seller = randomUUID();
const bidders = [randomUUID(), randomUUID(), randomUUID()];

let seed = process.env.MOCK_EMPTY ? [] : SEED;
if (process.env.MOCK_NO_LIVE) seed = seed.filter((lot) => lot.status !== "ACTIVE");

// Newest first, like the real list endpoint.
const auctions = seed.map((lot, index) => {
  // Later seed entries are the most recently listed.
  const created = startedAt - (seed.length - index) * 3_600_000;
  const startsAt = lot.status === "NOT_ACTIVE" ? startedAt + lot.startsIn : created;
  return {
    id: randomUUID(),
    owner_id: seller,
    item: { id: randomUUID(), name: lot.name, type: lot.type, description: lot.description },
    starting_price: lot.start,
    min_increment: lot.increment,
    current_bid: lot.bid,
    current_bidder_id: lot.bid === null ? null : bidders[index % bidders.length],
    bid_count: lot.bids,
    extensions: 0,
    settled_at: lot.status === "COMPLETED" && lot.bid !== null ? new Date(startedAt + lot.endsIn + 1000).toISOString() : null,
    starts_at: new Date(startsAt).toISOString(),
    ends_at: new Date(startedAt + lot.endsIn).toISOString(),
    status: lot.status,
    created_at: new Date(created).toISOString(),
    updated_at: new Date(created).toISOString(),
    // Not part of the REST response; used for trending and the live snapshot.
    _recent: lot.recent,
    _version: 1,
  };
});

auctions.sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));

const publicAuction = ({ _recent, _version, ...auction }) => auction;

const minNextBid = (a) => (a.current_bid === null ? Math.max(a.starting_price, 1) : a.current_bid + a.min_increment);

const snapshot = (a) => ({
  id: a.id,
  status: a.status,
  current_bid: a.current_bid,
  current_bidder_id: a.current_bidder_id,
  bid_count: a.bid_count,
  min_next_bid: minNextBid(a),
  ends_at: a.ends_at,
  extensions: a.extensions,
  version: a._version,
});

// ---------------------------------------------------------------- HTTP

function send(res, status, body, origin) {
  const headers = { "Content-Type": "application/json" };
  if (origin && ORIGINS.includes(origin)) {
    headers["Access-Control-Allow-Origin"] = origin;
    headers["Vary"] = "Origin";
    headers["Access-Control-Expose-Headers"] = "X-Request-ID, RateLimit-Limit, RateLimit-Remaining, Retry-After";
  }
  res.writeHead(status, headers);
  res.end(JSON.stringify(body));
}

const STATUSES = new Set(["NOT_ACTIVE", "ACTIVE", "COMPLETED", "CANCELLED"]);

function handle(req, res) {
  const origin = req.headers.origin;
  const url = new URL(req.url, `http://${req.headers.host}`);
  const path = url.pathname;

  if (req.method !== "GET") return send(res, 405, { error: "the mock API is read-only" }, origin);
  if (path === "/v1/healthcheck" || path === "/livez") return send(res, 200, { status: "available" }, origin);

  if (path === "/v1/auctions/trending") {
    if (FAIL.has("trending")) return send(res, 500, { error: "internal server error" }, origin);
    const limit = clamp(url.searchParams.get("limit"), 1, 50, 10);
    const trending = auctions
      .filter((a) => a.status === "ACTIVE" && a._recent > 0)
      .sort((a, b) => b._recent - a._recent)
      .slice(0, limit);
    return send(res, 200, { auctions: trending.map(publicAuction) }, origin);
  }

  if (path === "/v1/auctions") {
    if (FAIL.has("list")) return send(res, 500, { error: "internal server error" }, origin);
    const status = url.searchParams.get("status");
    if (status !== null && !STATUSES.has(status)) {
      return send(res, 400, { error: "invalid input", fields: { status: "is not a valid status" } }, origin);
    }
    const limit = clamp(url.searchParams.get("limit"), 1, 100, 20);
    const offset = decodeCursor(url.searchParams.get("cursor"));
    const all = auctions.filter((a) => status === null || a.status === status);
    const page = all.slice(offset, offset + limit);
    const next = offset + limit < all.length ? Buffer.from(String(offset + limit)).toString("base64url") : null;
    return send(res, 200, { auctions: page.map(publicAuction), next_cursor: next }, origin);
  }

  const one = /^\/v1\/auctions\/([0-9a-f-]{36})$/.exec(path);
  if (one) {
    const auction = auctions.find((a) => a.id === one[1]);
    if (!auction) return send(res, 404, { error: "auction not found" }, origin);
    return send(res, 200, publicAuction(auction), origin);
  }

  return send(res, 404, { error: "not found" }, origin);
}

function clamp(raw, min, max, fallback) {
  const n = Number(raw);
  return raw !== null && Number.isInteger(n) ? Math.min(max, Math.max(min, n)) : fallback;
}

function decodeCursor(raw) {
  if (!raw) return 0;
  const n = Number(Buffer.from(raw, "base64url").toString());
  return Number.isInteger(n) && n > 0 ? n : 0;
}

const server = createServer((req, res) => {
  if (DELAY > 0) setTimeout(() => handle(req, res), DELAY);
  else handle(req, res);
});

// ---------------------------------------------------------------- WebSocket

const wss = new WebSocketServer({ noServer: true });
const rooms = new Map(); // auction id -> Set<socket>

server.on("upgrade", (req, socket, head) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  const origin = req.headers.origin;
  if (url.pathname !== "/v1/ws" || (origin && !ORIGINS.includes(origin))) {
    socket.write("HTTP/1.1 403 Forbidden\r\n\r\n");
    socket.destroy();
    return;
  }
  wss.handleUpgrade(req, socket, head, (ws) => {
    ws.rooms = new Set();
    ws.on("message", (data) => {
      let command;
      try {
        command = JSON.parse(String(data));
      } catch {
        return ws.send(JSON.stringify({ type: "error", error: "invalid message" }));
      }
      const id = command.auction_id;
      if (command.action === "subscribe") {
        if (ws.rooms.size >= 20) return ws.send(JSON.stringify({ type: "error", error: "too many subscriptions" }));
        ws.rooms.add(id);
        if (!rooms.has(id)) rooms.set(id, new Set());
        rooms.get(id).add(ws);
        ws.send(JSON.stringify({ type: "subscribed", auction_id: id }));
      } else if (command.action === "unsubscribe") {
        ws.rooms.delete(id);
        rooms.get(id)?.delete(ws);
        ws.send(JSON.stringify({ type: "unsubscribed", auction_id: id }));
      } else {
        ws.send(JSON.stringify({ type: "error", error: "unknown action" }));
      }
    });
    ws.on("close", () => ws.rooms.forEach((id) => rooms.get(id)?.delete(ws)));
  });
});

function publish(auction, cause) {
  auction._version += 1;
  auction.updated_at = new Date().toISOString();
  const message = JSON.stringify({ type: "auction.updated", cause, auction: snapshot(auction) });
  rooms.get(auction.id)?.forEach((ws) => ws.readyState === 1 && ws.send(message));
}

// ---------------------------------------------------------------- lifecycle and simulated bids

setInterval(() => {
  const now = Date.now();
  for (const auction of auctions) {
    if (auction.status === "NOT_ACTIVE" && Date.parse(auction.starts_at) <= now) {
      auction.status = "ACTIVE";
      publish(auction, "auction.activated");
    } else if (auction.status === "ACTIVE" && Date.parse(auction.ends_at) <= now) {
      auction.status = "COMPLETED";
      if (auction.current_bid !== null) auction.settled_at = new Date().toISOString();
      publish(auction, "auction.completed");
    }
  }
}, 1000);

function simulateBid() {
  const now = Date.now();
  const candidates = auctions.filter((a) => a.status === "ACTIVE" && a._recent > 0 && Date.parse(a.ends_at) > now);
  if (candidates.length > 0) {
    // Busier lots attract more bids.
    const total = candidates.reduce((sum, a) => sum + a._recent, 0);
    let pick = Math.random() * total;
    const auction = candidates.find((a) => (pick -= a._recent) <= 0) ?? candidates[0];

    auction.current_bid = minNextBid(auction) + auction.min_increment * Math.floor(Math.random() * 3);
    auction.current_bidder_id = bidders[Math.floor(Math.random() * bidders.length)];
    auction.bid_count += 1;
    auction._recent += 1;
    // Anti-sniping, as in internal/bidding/decide.go.
    if (Date.parse(auction.ends_at) - now < SNIPE_WINDOW && auction.extensions < MAX_EXTENSIONS) {
      auction.ends_at = new Date(now + SNIPE_WINDOW).toISOString();
      auction.extensions += 1;
    }
    publish(auction, "bid.placed");
  }
  setTimeout(simulateBid, 4000 + Math.random() * 5000);
}
if (!process.env.MOCK_QUIET) setTimeout(simulateBid, 5000);

server.listen(PORT, () => {
  console.log(`mock auctionEngine API on http://localhost:${PORT} (${auctions.length} auctions)`);
});
