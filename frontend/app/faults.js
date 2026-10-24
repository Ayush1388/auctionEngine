// The faults an operator can switch on, in display order. The same names and
// words as internal/chaos on the Go side, so the real API and the demo engine
// are driven by one page.
export const FAULTS = [
  { name: "redis", label: "Redis", short: "Cache, trending, rate-limit buckets and fan-out",
    does: "Every Redis command fails.",
    degrades: "Reads go to PostgreSQL, trending is computed there, rate limits fail open, and live updates reach only this instance." },
  { name: "kafka", label: "Kafka", short: "Event stream and the async bid queue",
    does: "Kafka is unreachable.",
    degrades: "Synchronous bids are untouched. Events and queued bids wait in the outbox, in order, and flow again on recovery." },
  { name: "elasticsearch", label: "Elasticsearch", short: "Typo-tolerant search and type-ahead",
    does: "Every Elasticsearch call fails.",
    degrades: "After a few failures the breaker opens and search is answered from PostgreSQL in milliseconds, without typo tolerance." },
  { name: "bidding", label: "Bidding service", short: "The service that decides bids",
    does: "The bidding service is down.",
    degrades: "A few bids fail slowly, then the breaker opens and bids answer 503 at once instead of piling up. Nothing is charged." },
  { name: "postgres_slow", label: "Slow PostgreSQL", short: "The source of truth, answering late",
    does: "Every statement is delayed.",
    degrades: "Bids hold the row lock longer and queue behind each other, so latency climbs. Correctness is unchanged." },
];

export const faultByName = Object.fromEntries(FAULTS.map(f => [f.name, f]));
