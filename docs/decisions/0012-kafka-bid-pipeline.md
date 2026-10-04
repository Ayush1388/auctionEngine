# 0012. Asynchronous bids through Kafka, partitioned by auction

- **Status:** Accepted
- **Date:** 2026-10

## Context
Synchronous bidding (v0.3) is correct but, on a hot auction, every request waits inside PostgreSQL for the same row lock while holding an HTTP request and a database connection. Throughput for one auction is bounded by how fast those transactions serialise (~290 bids/s measured), and under a spike the waiting requests exhaust the connection pool. We also want a durable event stream that other services (analytics, notifications) can consume without touching our database.

## Decision
- **Opt-in async API:** `POST …/bids` with `Prefer: respond-async` (RFC 7240) records a `bid_requests` row **and** a `bid.requested` outbox event in one transaction, then returns `202` with a status URL. Without the header, or without Kafka, bids stay synchronous.
- **Outbox → Kafka relay:** the outbox worker produces each event with `ProduceSync` (acks=all, idempotent producer) and only marks it processed after the broker acknowledges. There is never a direct dual write.
- **Key = `auction_id`:** one auction's commands share a partition, so they're totally ordered. Different auctions spread over 12 partitions and are processed in parallel.
- **Consumer group `bid-workers`:** each partition is owned by one worker and processed sequentially; partitions run concurrently. Workers run inside the API (`BID_WORKERS`) or standalone (`cmd/bidworker`).
- **At-least-once + idempotency = effectively once:** offsets are committed after processing; the worker calls `PlaceBid` with key `req:<request_id>`, and outcomes are written only over `PENDING`.
- **Errors:** business rejections are outcomes (`REJECTED`); technical errors are retried in place with backoff (the partition waits, preserving order); after 5 attempts the command goes to `auction-bids-dlq` and the request becomes `FAILED`.
- **Rebalancing:** `BlockRebalanceOnPoll` and `AllowRebalance` after commit, so a partition is never processed by two workers and committed work is never repeated by the next owner.
- **Every auction event is also relayed to `auction-events`** for downstream consumers.

## Alternatives considered
- **Keep only synchronous bids:** simplest, and still the default. Async exists for spikes and for learning the pipeline.
- **Produce to Kafka from the handler:** a dual write. Kafka could have the command while the request row rolled back, or the reverse.
- **Kafka transactions (exactly-once semantics):** exactly-once only holds Kafka-to-Kafka. Our side effect is a PostgreSQL write, so idempotent processing is needed anyway.
- **One partition per auction:** unbounded partitions and metadata. Keying spreads auctions over a fixed set of partitions with the same ordering guarantee.

## Consequences
- Async clients see a result after a short delay and must poll or watch the WebSocket feed; a queued bid can still be rejected.
- Per-auction throughput is now bounded by one worker's sequential speed, but without lock waits, with no HTTP requests held open, and with the queue absorbing spikes.
- Increasing partitions later remaps keys; the in-flight ordering per auction would briefly not hold during the change.
