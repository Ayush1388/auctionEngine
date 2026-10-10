// The backend tour: the order in which a bid meets the system, shared by the tour
// page and by every demo page, so each demo says where it sits in the flow.
import { html, $ } from "./util.js";

export const STAGES = [
  {
    id: "request", n: 1, title: "The request arrives", page: "trust.html?part=request", short: "Gateway",
    nodes: ["browser", "gateway", "redis"],
    what: "A bid is a POST with a token and an Idempotency-Key. The gateway checks who you are, sheds bursts with a token bucket, and recognises a retry so a lost response never means a second bid.",
    tech: ["JWT access tokens", "Rotating refresh tokens", "Argon2id", "Token bucket in Redis (Lua)", "Idempotency-Key"],
    why: "Short tokens limit a leak, and a limiter in Redis is shared by every API instance. Redis is only an accelerator: if it dies the limiter fails open.",
    adr: "ADR 0008, 0009",
    demo: "Send one bid five times with the same key, then drain the rate-limit bucket.",
  },
  {
    id: "decision", n: 2, title: "One price, one decision", page: "race.html", short: "Bidding",
    nodes: ["gateway", "bidding", "postgres"],
    what: "Many people bid on one price at the same instant. Every bid is one transaction that locks the auction row first, decides on data nobody else can change, and writes everything together. A maximum bid is answered inside that same transaction.",
    tech: ["PostgreSQL row locks (FOR UPDATE)", "Pessimistic and optimistic strategies", "Proxy (maximum) bidding", "Anti-sniping", "gRPC bidding service"],
    why: "A global lock order (auction, then wallets by user ID) makes deadlocks impossible by construction. Optimistic locking halves tail latency but accepted bids stay the same, because one hot price is the real limit.",
    adr: "ADR 0006, 0013",
    demo: "Race several bidders on one price and compare the two locking strategies.",
  },
  {
    id: "money", n: 3, title: "The money is a ledger", page: "trust.html?part=money", short: "Wallet and ledger",
    nodes: ["bidding", "postgres"],
    what: "Holding, releasing and paying are double-entry journals that always sum to zero. CHECK constraints make a negative wallet impossible even if the code were wrong, and an audit recomputes every balance from the ledger.",
    tech: ["Double-entry append-only ledger", "CHECK (balance >= 0)", "Reconcile audit", "Settlement on close"],
    why: "Balances are a projection; the ledger is the truth. That is what lets the page below prove nothing was created or destroyed.",
    adr: "ADR 0007",
    demo: "Fire 50 bids from one wallet at once and watch the books balance.",
  },
  {
    id: "events", n: 4, title: "Events leave through the outbox", page: "system.html", short: "Outbox, Kafka, WebSocket",
    nodes: ["postgres", "outbox", "kafka", "redis", "websocket", "browser"],
    what: "The bid and its event commit together, so one cannot exist without the other. A worker then delivers the event at least once: to Kafka for the stream, to Redis pub/sub for every API instance, and over the WebSocket to every open screen.",
    tech: ["Transactional outbox", "SKIP LOCKED workers", "Kafka, partitioned by auction", "Redis pub/sub", "WebSocket with versioned snapshots"],
    why: "No dual write: the database is the only place a decision is made, and everything else is a consequence that can be retried.",
    adr: "ADR 0001, 0003, 0011, 0012",
    demo: "Place a bid and watch it travel; see which Kafka lane each lot is pinned to.",
  },
  {
    id: "search", n: 5, title: "Search is a read model", page: "searchlab.html", short: "Search",
    nodes: ["outbox", "elasticsearch", "postgres"],
    what: "Cars are indexed into Elasticsearch from the same outbox events. Search forgives typos. When Elasticsearch fails, a circuit breaker opens and the same query is answered from PostgreSQL full-text search.",
    tech: ["Elasticsearch", "PostgreSQL full-text fallback", "Circuit breaker", "Type-ahead"],
    why: "Search can be rebuilt from the database at any time, so losing it costs quality, never data.",
    adr: "ADR 0010",
    demo: "Search with a typo through both backends side by side.",
  },
  {
    id: "resilience", n: 6, title: "When something breaks", page: "chaos.html", short: "Resilience",
    nodes: ["redis", "kafka", "elasticsearch", "bidding", "postgres"],
    what: "Every dependency except PostgreSQL degrades instead of failing the request. Breakers fail fast, rate limits fail open, queued work waits in the outbox. Readiness separates a broken instance from a healthy one.",
    tech: ["Circuit breakers", "Fail-open limiter", "Readiness and liveness", "Graceful drain", "Fault injection"],
    why: "The question every system gets asked is what happens when X dies. Here you can kill X and read the answer.",
    adr: "ADR 0015, 0009",
    demo: "Kill Redis, Kafka, Elasticsearch, bidding or Postgres and keep bidding.",
  },
  {
    id: "observe", n: 7, title: "Seeing inside", page: "status.html", short: "Observability",
    nodes: ["gateway", "bidding"],
    what: "Each bid answers with Server-Timing and a trace ID. Prometheus metrics, health probes and pprof show the system from outside, and the status page reads the same series.",
    tech: ["Prometheus metrics", "OpenTelemetry traces", "Server-Timing", "pprof", "Structured logs with request IDs"],
    why: "A bid can be followed from the click to the screen with numbers that say where they came from.",
    adr: "ADR 0014",
    demo: "Watch bids per second and latency move, then open Behind the bid on any lot.",
  },
  {
    id: "beyond", n: 8, title: "Beyond English auctions", page: "formats.html", short: "Other formats",
    nodes: [],
    what: "The engine sells English (ascending) auctions with maximum bids. Dutch and sealed-bid fit the same money rules: hold, release, settle. They run in the browser to show what would change.",
    tech: ["Dutch (falling price)", "Sealed first price", "Sealed second price"],
    why: "Different formats move the race condition: from many bids on one price to many buyers at one tick, or to one close reading every bid.",
    adr: "demo only",
    demo: "Buy at a falling price, then seal a bid under both pricing rules.",
  },
];

export const stageById = Object.fromEntries(STAGES.map(s => [s.id, s]));

/**
 * A strip at the top of a demo page: where this stage sits in the flow, what it is
 * in one sentence, what it is built with, and the way to the neighbours.
 */
export function mountTourBar(id, { root = $("main") } = {}) {
  const s = stageById[id];
  if (!s || !root) return;
  const prev = STAGES[s.n - 2], next = STAGES[s.n];
  const el = document.createElement("nav");
  el.className = "tourbar";
  el.setAttribute("aria-label", "Backend tour");
  el.innerHTML = String(html`
    <div class="tourbar-top">
      <a class="tourbar-home" href="tour.html">Backend tour</a>
      <ol class="tourbar-steps" aria-label="Stages">${STAGES.map(x => html`<li><a href="${x.page}" ${x.id === id ? html`aria-current="step"` : ""} title="${x.title}"><span class="visually">Stage </span>${x.n}</a></li>`)}</ol>
      <span class="tourbar-nav">
        ${prev ? html`<a class="linkish" href="${prev.page}">Previous: ${prev.short}</a>` : ""}
        ${next ? html`<a class="linkish" href="${next.page}">Next: ${next.short}</a>` : html`<a class="linkish" href="tour.html">Back to the tour</a>`}
      </span>
    </div>
    <div class="tourbar-body">
      <p class="tourbar-what"><b>Stage ${s.n} of ${STAGES.length}. ${s.title}.</b> ${s.what}</p>
      <ul class="chips tourbar-tech" aria-label="Built with">${s.tech.map(t => html`<li>${t}</li>`)}</ul>
    </div>`);
  root.prepend(el);
}
