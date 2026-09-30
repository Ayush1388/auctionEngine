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

### Auth and accounts

- Registration with request validation and **Argon2id** password hashing
- **Account activation**: only a hash of the token is stored in the database, and tokens expire after 24 hours; a resend endpoint is included
- **JWT** login and an authentication middleware protecting private routes

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
| `POST` | `/v1/users/login` | | Get a JWT |
| `GET` | `/v1/users/me` | JWT | Current user |
| `POST` | `/v1/auctions` | JWT | List an item for auction |
| `GET` | `/v1/auctions` | optional | List auctions, newest first: `?status=ACTIVE&owner=me&limit=20&cursor=…` |
| `GET` | `/v1/auctions/{id}` | | Get one auction with its item |
| `POST` | `/v1/auctions/{id}/cancel` | JWT (owner) | Cancel an auction before it starts |
| `POST` | `/v1/auctions/{id}/bids` | JWT | Place a bid (send an `Idempotency-Key` header) |
| `GET` | `/v1/auctions/{id}/bids` | | Bid history, newest first |
| `GET` | `/v1/wallet` | JWT | Available and reserved balance |
| `POST` | `/v1/wallet/deposits` | JWT | Add test funds (`Idempotency-Key` required) |
| `GET` | `/v1/wallet/ledger` | JWT | Every money movement, newest first |

Errors are always JSON: `{"error": "..."}`, plus a `fields` map for validation errors:

```json
{"error": "invalid input", "fields": {"ends_at": "must be after starts_at", "item.name": "is required"}}
```

Prices are integers in the smallest currency unit (paise/cents).

## Running locally

```bash
# 1. Start Postgres
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
internal/auction    auctions: state machine, service, repository, lifecycle worker, pagination
internal/bidding    placing bids (two locking strategies), bid history, settlement
internal/wallet     wallets, double-entry ledger, reconciliation
internal/user       users: service, repository, Argon2id, activation tokens, JWT
internal/auth       JWT middleware (required and optional)
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
- **Later**: real-time updates over WebSockets, load tests

Design decisions are recorded in [`docs/decisions/`](docs/decisions/), and the development workflow is in [`docs/WORKFLOW.md`](docs/WORKFLOW.md).
