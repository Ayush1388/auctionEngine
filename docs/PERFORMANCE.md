# Performance and load testing

This page explains how the system was measured, what the measurements showed, and what was changed because of them. All numbers below were measured, not estimated. They come from a small machine, so read them as relative, before/after numbers, not as capacity claims.

## Environment

| | |
|---|---|
| Machine | 2 vCPU (Intel Xeon @ 2.8 GHz), shared by the API, PostgreSQL 18, Redis 7 **and** the load generator |
| API | one `cmd/api` instance, `RATE_LIMITS=off`, pessimistic locking unless noted |
| Data | 100 bidders with funded wallets, 20 ACTIVE auctions |
| Load | `cmd/loadgen`, 32 closed-loop workers, 20 s. Mix: 60% bids, 30% auction reads, 10% searches. 50% of bids go to one "hot" auction |

The generator competes with the server for the same 2 cores, so absolute throughput is understated. The comparisons between runs are fair because every run had the same handicap.

## Tools

- **`cmd/loadgen`**: seeds data, logs every user in through the API, then runs the mix and prints p50/p90/p99/max latency and status codes per operation. It is a closed loop (see *coordinated omission* below), and it honours `Retry-After` the way a well-behaved client would.
- **`loadtest/k6/auction.js`**: an open model at a fixed arrival rate, with SLO thresholds, for release gating. Run `go run ./cmd/loadgen -seed-only loadtest/k6/seed.json` first. k6 was not available in this environment, so the k6 script has no results here.
- **Go benchmarks**: `go test -run '^$' -bench . ./internal/...` (the bidding benchmarks need `TEST_DATABASE_URL`).
- **Metrics** on the admin port (`ADMIN_ADDR`) during every run. The two bottlenecks below were found there, not in the load generator's output.

## Run 1: baseline (PostgreSQL only)

| op | req/s | p50 ms | p90 ms | p99 ms | codes |
|---|---:|---:|---:|---:|---|
| bid | 288 | 66.1 | 86.8 | 109.4 | 201×5123 422×654 |
| read | 142 | 53.4 | 71.0 | 90.5 | 200×2851 |
| search | 45 | 101.5 | 131.0 | 159.5 | 200×909 |
| **total** | **475** | | | | |

A 53 ms p50 for a primary-key read is far too slow. The metrics showed why:

```
auction_db_pool_max_connections    4
auction_db_pool_empty_acquire_total 10550   ← ≈ every request waited for a connection
```

### Finding 1: the connection pool was the bottleneck

pgxpool's default size is `max(4, CPUs)` = 4. With 32 concurrent clients, almost every request queued *inside our process* waiting for one of 4 connections, so most of the latency was waiting, not database work.

**Fix:** `database.DefaultMaxConns = 20` (overridable with `?pool_max_conns=`), plus `MinConns`, `MaxConnLifetime` and `MaxConnIdleTime`.

## Run 2: pool of 20

| op | req/s | p50 ms | p90 ms | p99 ms | codes |
|---|---:|---:|---:|---:|---|
| bid | 355 | 49.5 | 156.5 | 322.2 | 201×4572 422×2551 |
| read | 180 | **18.9** | 29.4 | 41.0 | 200×3605 |
| search | 64 | **35.0** | 51.4 | 76.9 | 200×1292 |
| **total** | **600** (+26%) | | | | |

Reads got 2.8× faster and searches 2.9× faster. **Bid tail latency got worse** (p99 109 → 322 ms), which is the interesting part:

- Half of all bids target one auction. With the pessimistic strategy they queue on that auction's row lock (`SELECT … FOR UPDATE`). A bigger pool doesn't shorten that queue. It moves the queue from our process into PostgreSQL, and each waiting bid now **holds a connection while it waits**.
- A hot row is a serialisation point. Throughput on it is bounded by one transaction at a time, whatever the pool size or core count.

## Run 3: optimistic locking (`BID_LOCKING=optimistic`)

| op | req/s | p50 ms | p90 ms | p99 ms | codes |
|---|---:|---:|---:|---:|---|
| bid | 434 | 56.6 | 112.6 | **159.5** | 201×4509 **422×4191** |
| read | 224 | 11.1 | 18.3 | 27.4 | 200×4485 |
| search | 77 | 20.2 | 31.4 | 47.5 | 200×1544 |
| **total** | **735** | | | | |

Optimistic locking never waits on a lock, so the bid p99 halved and nothing else is starved of connections. But **accepted bids stayed the same** (4509 vs 4572): the extra attempts were rejected as "too low" after re-reading a price that had already moved. On a hot auction, accepted bids per second are set by the auction itself. The strategy only decides whether excess bids wait (pessimistic) or are rejected quickly (optimistic). The Go benchmark shows the same thing:

```
BenchmarkPlaceBid/HotAuction/pessimistic   255 bids/s   0.70 accepted/op   → ~178 accepted/s
BenchmarkPlaceBid/HotAuction/optimistic    682 bids/s   0.26 accepted/op   → ~178 accepted/s
BenchmarkPlaceBid/Spread/pessimistic       392 bids/s   1.00 accepted/op
BenchmarkPlaceBid/Spread/optimistic        400 bids/s   1.00 accepted/op
```

With bids spread over 64 auctions (no hot row) the two strategies are equal.

## Run 4: with Redis (cache-aside)

| op | req/s | p50 ms | p90 ms | p99 ms | codes |
|---|---:|---:|---:|---:|---|
| bid | 365 | 54.8 | 157.1 | 328.3 | 201×4720 422×2603 |
| read | 182 | **1.9** | 4.8 | 12.9 | 200×3649 |
| search | 60 | 47.3 | 66.2 | 95.5 | 200×1207 |

The auction read p50 fell from 18.9 ms to 1.9 ms, and singleflight kept database loads to 20 for 3,649 reads. But the cache hit ratio was suspiciously high for auctions receiving hundreds of bids per second, which led to the next finding.

### Finding 2: the outbox could only deliver 5 events per second

```sql
SELECT count(*) FILTER (WHERE processed_at IS NULL) FROM outbox_events;   -- 18437
```

The outbox worker claimed **one batch of 10 events per 2-second tick**, at most 5 events/s. The API produced about 350 events/s (one `bid.placed` per bid). After 20 seconds, 18,000 events were waiting. Because the outbox drives everything downstream, all of it was minutes behind: WebSocket updates, cache invalidation (so the cache served stale prices, which explains the high hit ratio), trending, search indexing and the Kafka relay. No test had caught it, because tests drive the worker directly with `ProcessOnce`.

**Fixes:**
1. **Drain, don't tick:** after a full batch the worker claims the next one at once, and sleeps only when a batch comes back short. The batch size went from 10 to 100.
2. **LISTEN/NOTIFY** (migration 000016): an `AFTER INSERT … FOR EACH STATEMENT` trigger calls `pg_notify('outbox_events')`. PostgreSQL delivers it on COMMIT, so the worker wakes **about 5 ms after commit** (measured in `TestWorkerIsWokenByNotify`) instead of up to 2 s later. Polling stays as a fallback.
3. **Batched bookkeeping:** successful events are marked processed with one `UPDATE … WHERE id = ANY($1)` per batch instead of one round trip each.
4. Per-event logging moved from Info to Debug. At hundreds of events/s it was real cost.

### Run 5: after the fixes (Redis on)

| op | req/s | p50 ms | p90 ms | p99 ms |
|---|---:|---:|---:|---:|
| bid | 365 | 52.1 | 154.3 | 327.3 |
| read | 178 | 2.5 | 25.2 | 41.6 |
| search | 63 | 44.6 | 60.2 | 81.9 |

```
during load:   ~190 events/s delivered   (was 5/s)
after load:    backlog 3886 → 2786 → 1186 in 2 s   ≈ 1,300 events/s drain rate
```

The cache hit ratio dropped to a realistic 73%: invalidations now arrive on time. Under full load the single worker still trails the bid rate on this 2-core box, because it competes for CPU with everything else. The next step, if needed, is **partitioning the outbox by auction**: N workers, each claiming only `hashtext(aggregate_id) % N = k`. That keeps per-auction ordering while adding parallelism, the same idea as Kafka partitions.

## Run 6: overload and load shedding

128 workers, which is 4× the load above:

| | bid p50 / p99 ms | read p99 ms | search p50 ms | 503s |
|---|---|---|---|---|
| no limit | 254 / 545 | 270 | 420 | 0 |
| `MAX_IN_FLIGHT=48` | **83 / 364** | **76** | **110** | 15% (fast, with `Retry-After`) |

Without a limit, every request is accepted and every request is slow. With a limit, the excess is rejected in microseconds and the accepted requests keep near-normal latency. The number of accepted bids barely changed (3,467 vs 3,594). That trade is what load shedding is for.

**A mistake worth recording:** the first shedding run used a generator that retried a 503 immediately. It produced 9,500 req/s of rejections, and goodput *fell* because the CPU went into saying "no". Shedding only works if clients back off. That is why the API sends `Retry-After` and why `loadgen` now honours it with jitter.

## Micro-benchmarks

| benchmark | result | meaning |
|---|---|---|
| `breaker.BenchmarkDoClosed` | 63 ns/op | breaker overhead is ~0.01% of a 1 ms network call |
| `ratelimit.BenchmarkMemoryAllow` (10k keys) | 308 ns/op | in-process token bucket |
| `metrics.BenchmarkMiddleware` vs baseline | 1342 vs 587 ns/op | RED metrics add ~0.75 µs and 5 allocs per request |

## Coordinated omission

`cmd/loadgen` is a closed loop: a worker sends its next request only when the previous one returns. When the server stalls for 1 s, a closed-loop worker records **one** slow request. Real users, who keep arriving, would have produced hundreds of requests that each waited. Closed-loop percentiles therefore understate tail latency under overload. Use the k6 open-model script (`constant-arrival-rate`) to check SLOs. Its `dropped_iterations` threshold flags a generator that couldn't keep the rate.

## Reproducing

```bash
RATE_LIMITS=off ADMIN_ADDR=:9090 REDIS_URL=redis://localhost:6379/0 go run ./cmd/api
go run ./cmd/loadgen -duration 20s -concurrency 32 -json results.json
curl -s localhost:9090/metrics | grep -E 'auction_(db_pool|outbox|cache|bids_total)'
TEST_DATABASE_URL=… go test ./internal/bidding -run '^$' -bench PlaceBid -benchtime 3s
```
