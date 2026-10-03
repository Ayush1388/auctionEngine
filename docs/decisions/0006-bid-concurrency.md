# 0006. Serialising bids with row locks and a global lock order

- **Status:** Accepted
- **Date:** 2026-10

## Context
Many users bid on one auction at the same moment. Each bid reads the current bid, decides whether the new one is high enough, and writes. Done naively (read, check, write), two bids read the same current bid, both pass the check, and the last writer silently wins with a lower amount or a double reservation. A bid also moves money between two wallets (the new leader's and the previous leader's), so two bids on different auctions can touch the same wallets in opposite order and deadlock.

## Decision
- Every bid is one transaction.
- **Pessimistic (default):** `SELECT … FOR UPDATE OF a` locks the auction row before reading it. Bids on one auction queue on that lock; each decides on data that can't change until it commits.
- **Optimistic (configurable, `BID_LOCKING=optimistic`):** read without a lock, then `UPDATE … WHERE version = $read`. Zero rows means someone else bid first; the transaction is retried with jittered backoff, up to 8 times.
- **Lock order is global:** auction row first, then wallets in ascending user ID (`wallet.LockForUpdate`). Settlement follows the same order; the lifecycle worker only locks auctions. No two paths can wait on each other in a cycle.
- Funds are checked on the locked wallet row; `CHECK (available_amount >= 0)` is the backstop.

## Alternatives considered
- **SERIALIZABLE isolation with retries:** correct without explicit locks, but every conflicting transaction aborts late, after doing all its work.
- **An in-memory lock or a single goroutine per auction:** fast, but breaks the moment there are two API instances.
- **Redis/distributed lock:** another system that can fail, and still needs the database to be consistent.

## Consequences
- Throughput per auction is bounded by one bid per transaction round trip (~290 accepted bids/s locally; see `BenchmarkPlaceBid`). Different auctions proceed in parallel.
- Kafka partitioning by `auction_id` (v0.8) gives the same per-auction ordering at the queue level.
- Removing `FOR UPDATE`, or locking wallets in request order, is caught by `TestConcurrentBidsOnOneAuction` and `TestCrossAuctionBidsDoNotDeadlock`.
