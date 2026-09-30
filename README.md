# Auction Engine

[![CI](https://github.com/Ayush1388/auctionEngine/actions/workflows/ci.yml/badge.svg)](https://github.com/Ayush1388/auctionEngine/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18-4169E1?logo=postgresql&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)
![Status](https://img.shields.io/badge/status-in%20progress-orange)

A backend for **live auctions** written in Go, with user wallets, bid reservations and reliable event delivery.

The focus is on correctness under failure: a user's signup and their activation email can never get out of sync, background workers can't process the same job twice, and money can't go negative.

> 🚧 **In progress.** Users, auth and the outbox pipeline work end to end. Auction and bidding endpoints are being built on top of the existing schema.

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
- **Retry with delay:** failed events are rescheduled 30 seconds later, and `attempts` and `last_error` are recorded.
- A **partial index** on pending events (`WHERE processed_at IS NULL`) keeps polling cheap as the table grows.

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
- **Graceful shutdown**: on SIGINT/SIGTERM the HTTP server drains in-flight requests and the outbox worker stops cleanly, with a 10-second deadline
- A multi-stage **Docker** build that runs as a non-root user

---

## Architecture

```
            ┌──────────────┐     one transaction     ┌───────────────────────────┐
 HTTP  ───► │  handlers →  │ ──────────────────────► │ PostgreSQL                │
 client     │  services →  │  users +                │  users, wallets, auctions │
            │  repositories│  outbox_events          │  bids, outbox_events ...  │
            └──────────────┘                         └─────────────┬─────────────┘
                                                                   │ claim (SKIP LOCKED)
                                                     ┌─────────────▼─────────────┐
                                                     │ Outbox worker (goroutine) │ ──► SMTP
                                                     │ retry · lease · mark done │
                                                     └───────────────────────────┘
```

## API

| Method | Route | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/healthcheck` | | Service health |
| `POST` | `/v1/users/register` | | Create an account and queue the activation email |
| `GET` | `/v1/users/activate?token=…` | | Activate an account |
| `POST` | `/v1/users/resend-activation` | | Send a new activation token |
| `POST` | `/v1/users/login` | | Get a JWT |
| `GET` | `/v1/users/me` | JWT | Current user |

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
cmd/api          HTTP server entry point, wiring, graceful shutdown
cmd/migrate      migration runner CLI
internal/user    users: service, repository, Argon2id, activation tokens, JWT
internal/auth    JWT middleware
internal/outbox  outbox repository (claim / retry / mark processed) and worker
internal/email   SMTP service and outbox event handler
internal/auction auction domain (in progress)
internal/migration  migration runner (up/down, advisory lock)
internal/testdb  isolated, migrated schema per integration test
migrations/      versioned up/down SQL
```

## Roadmap

Work is tracked in [issues](https://github.com/Ayush1388/auctionEngine/issues) and grouped into milestones:

- **v0.1 – Users and auth** ✅
- **v0.2 – [Auction management](https://github.com/Ayush1388/auctionEngine/milestone/1)**: create, view, list and cancel auctions; lifecycle worker
- **v0.3 – Bidding**: wallet reservations, concurrent bids, settlement
- **Later**: real-time updates over WebSockets, load tests

Design decisions are recorded in [`docs/decisions/`](docs/decisions/), and the development workflow is in [`docs/WORKFLOW.md`](docs/WORKFLOW.md).
