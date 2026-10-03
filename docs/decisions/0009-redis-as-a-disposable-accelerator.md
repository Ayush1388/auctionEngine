# 0009. Redis as a disposable accelerator, never a source of truth

- **Status:** Accepted
- **Date:** 2026-10

## Context
Auction pages are read far more than they change; trending needs fast counting; rate limits must be shared across instances. Redis is excellent at all three, but it's in-memory, not transactional with PostgreSQL, and can lose data.

## Decision
Redis holds only data that is cheap to lose:
- **Cache-aside** for single auctions (`auctioncache`), 30 s TTL, deleted by outbox events on any change, with **singleflight** against stampedes.
- **Trending**: hourly sorted-set buckets combined with weighted `ZUNION`; fed by `bid.placed` through an idempotent Lua script (`SET NX` dedup + `ZINCRBY`).
- **Shared rate limits**: the token bucket as an atomic Lua script using Redis `TIME`.

Every feature degrades if Redis fails: cache → read PostgreSQL; trending → a PostgreSQL query; rate limits → fail open. Nothing that decides bids or money reads Redis. Redis is optional (`REDIS_URL`).

## Alternatives considered
- **Write-through cache:** keeps the cache warm but writes every change twice, including data nobody reads.
- **Invalidate inside each write path:** fresher, but every current and future write path must remember to do it; events cover them all.
- **Cache inside the bidding path:** faster bids, but a stale read there would accept a bid below the real current price.

## Consequences
- Reads can be up to one outbox delivery (≈2 s) stale; the bid response and the database are always exact.
- One more service to run, but not one more thing that can take the API down.
