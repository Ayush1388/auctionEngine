# Auction Engine

[![CI](https://github.com/Ayush1388/auctionEngine/actions/workflows/ci.yml/badge.svg)](https://github.com/Ayush1388/auctionEngine/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18-4169E1?logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)
![Status](https://img.shields.io/badge/status-in%20progress-orange)

A backend for **live auctions** written in Go, with user wallets, bid reservations and reliable event delivery.

The focus is on correctness under concurrency and failure: a user's signup and their activation email can never get out of sync, two workers can't move the same auction, a seller can't cancel an auction that has just gone live, and money can't go negative.

> 🚧 **In progress.** Users, auth, the outbox pipeline and auction management (v0.2) work end to end, with integration tests against Postgres. Bidding (v0.3) is next.

---

## Highlights

### Transactional outbox for reliable email

Registering a user and sending their activation email look like one action, but they touch two systems (Postgres and SMTP). If the email is sent directly from the request handler, an SMTP outage either loses the email or fails the signup.

Instead, the user row and an `outbox_events` row are written in **one database transaction**. A background worker then delivers the events:

```sql
WITH candidates AS (
    SELECT id FROM outbox_events
    WHERE processed_at IS NULL
      AND available_at <= now()
      AND (locked_at IS NULL OR locked_at < now() - INTERVAL '5 minutes')
    ORDER BY created_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED           -- many workers, no double processing
)
UPDATE outbox_events e
SET locked_at = now(), attempts = e.attempts + 1
FROM candidates WHERE e.id = candidates.id
RETURNING ...
```

- **`FOR UPDATE SKIP LOCKED`** lets several workers poll the same table without claiming the same event.
- **Lease timeout:** if a worker crashes mid-event, the lock expires after 5 minutes and another worker picks the event up.
- **Retry with backoff:** failed events are retried after 30s, 1m, 2m… (capped at 1h). After 8 attempts they are parked with `failed_at` instead of retrying forever. `attempts` and `last_error` are recorded.
- **Secrets don't linger:** the activation token is removed from the stored payload once the email has been sent.
- A **partial index** on pending events (`WHERE processed_at IS NULL`) keeps polling cheap as the table grows.

### Concurrent bidding without races, double spending or deadlocks

Every bid is one transaction: lock the auction row (`SELECT … FOR UPDATE`), lock the affected wallets **in user-ID order**, decide, then write the bid, the reservation, two ledger journals, the auction update and a `bid.placed` outbox event together. An optimistic strategy (version compare-and-swap with retries) can be switched on with `BID_LOCKING=optimistic` for comparison.

Tests prove it under load: 40 users bidding at once on one auction, one user trying to spend the same money on two auctions, two users outbidding each other on two auctions at the same instant (the deadlock case), and a bid racing the auction's end. Breaking the lock or the lock order makes those tests fail. See [`docs/decisions/0006`](docs/decisions/0006-bid-concurrency.md).

### Double-entry ledger

Money never changes in place. Every movement is a journal whose lines sum to zero, and the ledger tables are append-only (enforced by triggers). `wallet.Reconcile` recomputes all balances from the ledger. Settlement is idempotent, so a redelivered `auction.completed` event can't pay the seller twice. See [`docs/decisions/0007`](docs/decisions/0007-double-entry-ledger.md).

### Redis: cache, trending, shared rate limits

Redis only holds data that is cheap to lose ([`0009`](docs/decisions/0009-redis-as-a-disposable-accelerator.md)):

- **Cache-aside** auction reads with a 30 s TTL, invalidated by outbox events, and **singleflight** so 100 simultaneous cache misses cost one database query (tested).
- **Trending** from hourly sorted-set buckets, fed idempotently by a Lua script so redelivered events never double-count.
- **Token buckets as an atomic Lua script**, so every API instance shares one limit per client (60 concurrent requests across 3 instances with a burst of 10 let exactly 10 through).

If Redis goes down, the cache reads PostgreSQL, trending is computed from PostgreSQL, and rate limits fail open.

### Search: Elasticsearch as a read model

`GET /v1/auctions/search?q=vintage camra` tolerates typos, ranks title matches first and highlights what matched. `GET /v1/auctions/suggest?q=iph` autocompletes from edge n-grams.

- PostgreSQL stays the source of truth. The index is fed by **outbox events**, never written to directly by handlers.
- **External versioning** (`auctions.version`, bumped by a trigger on every update) means a late or duplicate event can't roll a document back.
- Search returns IDs and the auctions are loaded from PostgreSQL in **one query**, avoiding N+1 and showing exact prices.
- If Elasticsearch is down or not configured, **PostgreSQL full-text search** (generated `tsvector` + GIN index) answers instead.
- `go run ./cmd/admin reindex` rebuilds the index with **zero downtime** by swapping an alias.

See [`docs/decisions/0010`](docs/decisions/0010-search-as-a-read-model.md).

### gRPC: bidding as its own service

`cmd/biddingsvc` serves `BiddingService` (`api/proto/bidding/v1/bidding.proto`): `PlaceBid`, `ListBids`, and the server-streaming `WatchAuction`. With `BIDDING_GRPC_ADDR` set, the API becomes a gateway that calls it.

- The HTTP handlers depend on a `Bidder` interface, so the in-process service and the gRPC client are interchangeable, with no handler changes.
- Domain errors survive the network as gRPC codes with `ErrorInfo`/`BadRequest` details and come back as the same Go errors; an unreachable service becomes a **503**.
- **Deadlines propagate** into PostgreSQL: when the caller gives up, the query is cancelled (tested with a held row lock).
- **Retries are safe** because every call carries an idempotency key. Service-to-service auth, request-ID propagation, health checks, reflection and graceful stop are included.

What still shares a database, and the path to splitting it, is in [`docs/decisions/0013`](docs/decisions/0013-bidding-service-over-grpc.md).

### Kafka: ordered, asynchronous bid processing

`POST /v1/auctions/{id}/bids` with `Prefer: respond-async` returns **202** in milliseconds; the bid is placed by a worker, and the result is at `GET /v1/bid-requests/{id}` (and on the WebSocket feed).

```
request ─(1 tx: bid_requests + outbox)─► relay ─► Kafka auction-bids (key = auction_id, 12 partitions)
                                                      │ consumer group "bid-workers"
                                                      ▼ one worker per partition, records in order
                                                 bidding.PlaceBid (same rules and locks)
```

- **Partitioning by auction** gives a total order of bids per auction and parallelism across auctions. A test queues 30 strictly increasing bids on 4 interleaved auctions; any reordering would reject one, and a deliberately broken worker fails it.
- **At-least-once delivery + idempotent processing:** offsets are committed after the work; redelivered commands reuse the idempotency key, so a duplicate can't bid twice (tested).
- **Retries in place** keep order on transient errors; **dead-lettering** after 5 attempts keeps one poison message from blocking an auction.
- `cmd/bidworker` scales workers independently of the API. Every auction event is also streamed to `auction-events`.

See [`docs/decisions/0012`](docs/decisions/0012-kafka-bid-pipeline.md).

### Live updates over WebSockets

Connect to `GET /v1/ws`, send `{"action":"subscribe","auction_id":"…"}`, and every bid, extension or status change arrives as a snapshot within milliseconds. Try [`docs/examples/live-auction.html`](docs/examples/live-auction.html).

- The outbox gives an event to **one** instance, but viewers are connected to **all** of them, so the snapshot is fanned out through **Redis pub/sub** to every instance's hub.
- Each message carries the auction `version`, so clients drop out-of-order updates.
- **Backpressure:** each connection has a bounded buffer and a single writer goroutine; a client too slow to keep up is disconnected instead of stalling the room.
- **Heartbeats** detect dead connections; the origin allow-list blocks cross-site WebSocket hijacking; the connection cap sheds load with close code 1013.

See [`docs/decisions/0011`](docs/decisions/0011-websocket-fanout.md).

### Auction lifecycle as a state machine

```
NOT_ACTIVE ──(starts_at)──► ACTIVE ──(ends_at)──► COMPLETED
    │
    └──(owner cancels)──► CANCELLED
```

The allowed moves live in one map in `internal/auction/status.go`. The database updates derive their `WHERE status = ANY(...)` clause from it (`SourcesOf`), so the SQL and the rules can't drift apart. A test checks all 16 state pairs.

### Race-free status changes

Cancelling looks like "check the auction is still pending, then cancel it". Done as a SELECT followed by an UPDATE, the lifecycle worker could activate the auction in between, and a live auction would get cancelled. Instead the check **is** the update:

```sql
UPDATE auctions SET status = 'CANCELLED', updated_at = now()
WHERE id = $1 AND status = ANY($2) AND owner_id = $3 AND starts_at > $4
```

Postgres re-checks the `WHERE` clause on the locked row, so of two racing updates exactly one succeeds. A test races cancel against activation 25 times and asserts a single winner each round.

### Lifecycle worker

A background worker activates and completes auctions on schedule. It claims batches with `FOR UPDATE SKIP LOCKED`, so it's safe to run on every API instance; a test runs 4 workers over 200 auctions and checks each is completed exactly once. Completing an auction writes an `auction.completed` outbox event **in the same transaction**, for settlement to consume.

### Keyset pagination

`GET /v1/auctions` pages with a cursor on `(created_at, id)` instead of `OFFSET`, so page 500 costs the same as page 1 and new auctions arriving mid-scroll never cause duplicates. Each filter combination has a matching composite index, verified with `EXPLAIN`.

### Auth, sessions and abuse protection

- Registration with validation and **Argon2id** hashing (parameters stored per hash, upgraded on login)
- **Account activation** with hashed, expiring, single-use tokens
- **Short-lived JWT access tokens + rotating refresh tokens** with reuse detection and logout ([`0008`](docs/decisions/0008-access-and-refresh-tokens.md))
- **RBAC**: `user` and `admin` roles; admin-only operator endpoints; `go run ./cmd/admin promote <email>`
- **Token-bucket rate limiting** per IP, per user and per login email; `X-Forwarded-For` trusted only from configured proxies
- No account enumeration: same answers and timing whether an email exists or not
- Request IDs, structured access logs, panic recovery, CORS allow-list and security headers on every response
- **OpenAPI 3.1** contract at `/v1/openapi.json`, kept in sync with the router by a test

### Database

- A **hand-written migration runner** with versioned up/down SQL files, tracked in `schema_migrations`
- **`pgxpool`** connection pooling
- Integrity enforced in the schema itself: `CHECK (available_amount >= 0)` on wallets, `CHECK (ends_at > starts_at)` on auctions, status enums and foreign keys throughout
- Tables: `users`, `wallets`, `items`, `auctions`, `bids`, `bid_reservations`, `wallet_transactions`, `outbox_events`

### Operations

- Structured **JSON logging** with `log/slog`
- **Graceful shutdown**: on SIGINT/SIGTERM the HTTP server drains in-flight requests, then background workers finish their current event and stop, all within a 10-second deadline
- **CI** on every pull request: `gofmt`, `go vet`, and `go test -race` against Postgres 18
- A multi-stage **Docker** build that runs as a non-root user

---

## Architecture

```
            ┌──────────────┐    one transaction      ┌───────────────────────────┐
 HTTP  ───► │  handlers →  │ ──────────────────────► │ PostgreSQL                │
 client     │  services →  │  state change +         │  users, items, auctions   │
            │  repositories│  outbox_events          │  bids, outbox_events ...  │
            └──────────────┘                         └──────┬─────────────┬──────┘
                                         claim (SKIP LOCKED)│             │claim (SKIP LOCKED)
                                     ┌──────────────────────▼───┐   ┌─────▼────────────────────┐
                                     │ Lifecycle worker         │   │ Outbox worker            │ ──► SMTP
                                     │ activate · complete      │   │ retry · lease · mark done│
                                     └──────────────────────────┘   └──────────────────────────┘
```

Services own transaction boundaries; repositories accept anything that can run a query (pool or transaction) and never begin transactions themselves. See [`docs/decisions/0005`](docs/decisions/0005-transactions-owned-by-services.md).

## API

| Method | Route | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/healthcheck` | | Service health |
| `POST` | `/v1/users/register` | | Create an account and queue the activation email |
| `GET` | `/v1/users/activate?token=…` | | Activate an account |
| `POST` | `/v1/users/resend-activation` | | Send a new activation token (always `204`, so it can't be used to probe for accounts) |
| `POST` | `/v1/users/login` | | Get an access token and a refresh token |
| `POST` | `/v1/auth/refresh` | | Rotate the refresh token, get a new access token |
| `POST` | `/v1/auth/logout` | | Revoke the session |
| `GET` | `/v1/users/me` | JWT | Current user |
| `POST` | `/v1/auctions` | JWT | List an item for auction |
| `GET` | `/v1/auctions` | optional | List auctions, newest first: `?status=ACTIVE&owner=me&limit=20&cursor=…` |
| `GET` | `/v1/auctions/trending` | | Active auctions with the most bids in the last hour |
| `GET` | `/v1/auctions/search` | | Full-text search: `?q=…&status=&limit=&cursor=` |
| `GET` | `/v1/auctions/suggest` | | Type-ahead suggestions for active auctions: `?q=…` |
| `GET` | `/v1/auctions/{id}` | | Get one auction with its item (cached in Redis) |
| `POST` | `/v1/auctions/{id}/cancel` | JWT (owner) | Cancel an auction before it starts |
| `GET` | `/v1/ws` | | WebSocket: subscribe to live auction updates |
| `POST` | `/v1/auctions/{id}/bids` | JWT | Place a bid (send an `Idempotency-Key` header) |
| `GET` | `/v1/auctions/{id}/bids` | | Bid history, newest first |
| `GET` | `/v1/bid-requests/{id}` | JWT (owner) | Outcome of an async bid (`Prefer: respond-async`) |
| `GET` | `/v1/wallet` | JWT | Available and reserved balance |
| `POST` | `/v1/wallet/deposits` | JWT | Add test funds (`Idempotency-Key` required) |
| `GET` | `/v1/wallet/ledger` | JWT | Every money movement, newest first |
| `GET` | `/v1/admin/reconcile` | admin | Check the books balance |
| `GET` | `/v1/admin/outbox/failed` | admin | Dead-lettered events |
| `POST` | `/v1/admin/outbox/{id}/retry` | admin | Requeue a dead-lettered event |
| `GET` | `/v1/openapi.json` | | OpenAPI 3.1 description of this API |

Errors are always JSON: `{"error": "..."}`, plus a `fields` map for validation errors:

```json
{"error": "invalid input", "fields": {"ends_at": "must be after starts_at", "item.name": "is required"}}
```

Prices are integers in the smallest currency unit (paise/cents).

## Running locally

```bash
# 1. Start Postgres (and Redis, optional)
docker compose up -d

# 2. Configure (create a .env file in the project root)
PORT=4000
ENVIRONMENT=development
DATABASE_URL=postgres://auction:auction@localhost:5432/auction?sslmode=disable
JWT_SECRET=change-me
JWT_ISSUER=auction-engine
JWT_EXPIRATION_HOURS=24
SMTP_HOST=sandbox.smtp.mailtrap.io
SMTP_PORT=2525
SMTP_USERNAME=...
SMTP_PASSWORD=...
SMTP_FROM=no-reply@auction.local
APP_BASE_URL=http://localhost:4000
REDIS_URL=redis://localhost:6379/0   # optional
ELASTICSEARCH_URL=http://localhost:9200   # optional
KAFKA_BROKERS=localhost:9092              # optional; enables async bids
BIDDING_GRPC_ADDR=localhost:50051         # optional; call cmd/biddingsvc over gRPC
INTERNAL_TOKEN=change-me-too              # shared secret between gateway and services

# 3. Run migrations, then the API
go run ./cmd/migrate
go run ./cmd/api

# Roll back the latest migration
go run ./cmd/migrate -down 1
```

### Tests

Unit tests run anywhere. Integration tests need Postgres and are skipped unless `TEST_DATABASE_URL` is set; each test gets its own freshly migrated schema, which is dropped afterwards.

```bash
go test ./...                                   # unit tests only

export TEST_DATABASE_URL=postgres://auction:auction@localhost:5432/auction?sslmode=disable
go test -race ./...                             # unit + integration
```

CI runs formatting checks, `go vet` and the full test suite against Postgres on every pull request.

## Project layout

```
cmd/api             HTTP server entry point, wiring, graceful shutdown
cmd/migrate         migration CLI (up, or -down N)
cmd/admin           operator CLI (promote / demote admins)
internal/auction    auctions: state machine, service, repository, lifecycle worker, pagination
internal/bidding    placing bids (two locking strategies), bid history, settlement
internal/wallet     wallets, double-entry ledger, reconciliation
internal/user       users: service, repository, Argon2id, activation tokens, JWT
internal/auth       JWT middleware (required, optional, RequireRole)
internal/session    refresh tokens: issue, rotate, reuse detection, logout
internal/ratelimit  token-bucket limiter, middleware, trusted-proxy client IP
internal/middleware request IDs, access log, recover, security headers, CORS
internal/logctx     request-scoped logger in the context
internal/redisx     Redis client and conventions
internal/auctioncache  cache-aside auction reads, singleflight, event invalidation
internal/trending   hourly sorted-set ranking with Postgres fallback
internal/search     Elasticsearch + PostgreSQL full-text backends, indexer, reindex
internal/realtime   WebSocket hub, rooms, heartbeats, backpressure, Redis fan-out
internal/kafkax     topics, idempotent producer, outbox → Kafka relay
internal/bidqueue   async bid requests and the consumer-group bid worker
cmd/bidworker       bid workers without the HTTP API
cmd/biddingsvc      the bidding service (gRPC)
api/proto/          protobuf contracts; generated code in internal/gen (make proto)
internal/grpcsvc    gRPC server, client, interceptors, error mapping
tools/protogen      pure-Go protoc replacement used by make proto
api/                OpenAPI 3.1 document (embedded, served at /v1/openapi.json)
internal/outbox     outbox repository, worker and event router
internal/email      SMTP service and outbox event handler
internal/database   pgx pool, DBTX interface, WithTx
internal/httpx      JSON request/response helpers and error format
internal/validation per-field validation messages
internal/migration  migration runner (up/down, advisory lock)
internal/testdb     isolated, migrated schema per integration test
migrations/         versioned up/down SQL
docs/decisions/     why things are built the way they are
```

## Roadmap

Work is tracked in [issues](https://github.com/Ayush1388/auctionEngine/issues) and grouped into milestones:

- **v0.1 – Users and auth** ✅
- **v0.2 – [Auction management](https://github.com/Ayush1388/auctionEngine/milestone/1)** ✅: create, view, list and cancel auctions; lifecycle worker
- **v0.3 – Bidding** ✅: wallet reservations, concurrent bids, double-entry ledger, settlement, idempotency, anti-sniping
- **v0.4 – Security** ✅: rate limiting, refresh tokens, RBAC, CORS, request IDs, OpenAPI
- **v0.5 – Redis** ✅: cache-aside, stampede protection, trending, shared rate limits
- **v0.6 – Search** ✅: Elasticsearch read model, typo tolerance, autocomplete, PostgreSQL fallback, zero-downtime reindex
- **v0.7 – Real-time** ✅: WebSocket rooms, heartbeats, backpressure, multi-instance fan-out
- **v0.8 – Kafka** ✅: outbox relay, bids partitioned by auction, consumer groups, idempotent consumers, DLQ
- **v0.9 – gRPC** ✅: bidding service, streaming, deadlines, interceptors, error details, safe retries
- **Later**: production hardening

Design decisions are recorded in [`docs/decisions/`](docs/decisions/), and the development workflow is in [`docs/WORKFLOW.md`](docs/WORKFLOW.md).
