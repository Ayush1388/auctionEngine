# 0015. Resilience: circuit breakers, load shedding, bounded SMTP, outbox draining

- **Status:** Accepted
- **Date:** 2026-10

## Context
Every remote call can hang or fail: Elasticsearch, the bidding service, SMTP and PostgreSQL. Deadlines (v0.9) stop a single call from hanging forever. They don't stop the *aggregate* failure: during an outage every request still waits for its full timeout, goroutines and connections pile up, and the failure spreads into a slow or dead API (a cascading failure). The v1.0 load test (`docs/PERFORMANCE.md`) also found two real capacity bugs: a 4-connection DB pool, and an outbox worker capped at 5 events/s.

## Decision
- **Circuit breakers (`internal/breaker`).** Closed → open after `Threshold` consecutive failures → half-open after `Cooldown`, which lets `HalfOpenMax` trial calls through → closed after `HalfOpenSuccesses`. Each dependency decides what counts as a failure (`IsFailure`): a bid that is too low, a bad cursor or the caller's own cancellation is not the dependency failing. The state is exported as `auction_circuit_breaker_state{name}`.
  - **Elasticsearch** (`search.Guarded`): when open, searches skip straight to PostgreSQL, and indexing fails fast so the outbox retries later.
  - **Bidding gRPC client**: when open, the gateway answers 503 + `Retry-After` immediately.
  - **SMTP**: when open, sends fail fast and the outbox backs off.
- **SMTP with deadlines.** `smtp.SendMail` has no timeout at all. A server that accepts TCP and goes silent would block the outbox worker, and so every event behind the email, forever. It is replaced with a context-aware conversation (dial with context, connection deadline, STARTTLS, AUTH), bounded to 30 s. Tested with a silent server.
- **Load shedding (`middleware.LimitInFlight`, `MAX_IN_FLIGHT`).** A semaphore. Above the limit, a request gets 503 + `Retry-After: 1` at once. Probes and WebSockets are exempt.
- **DB pool sizing.** Default 20 connections, `MinConns` 2, connection lifetime and idle limits. The URL can override it.
- **Outbox throughput.** Drain back to back while batches are full; batch size 100; LISTEN/NOTIFY wake-up from an `AFTER INSERT` statement trigger; one `UPDATE … = ANY($1)` per batch.

## Consequences
- An outage of an optional dependency costs milliseconds per request instead of seconds, and the dependency gets breathing room to recover.
- Live updates arrive about 5 ms after commit instead of up to 2 s later.
- Breakers add state to reason about. A breaker that is too sensitive turns blips into outages, which is why only consecutive dependency failures count.
- The outbox stays single-worker per process to keep per-auction ordering. Partitioning by auction is the next step if one worker isn't enough.

## Alternatives considered
- **Retries alone:** they make an outage worse (more load on a failing service).
- **A library (sony/gobreaker, failsafe-go):** fine choices. The state machine is about 150 lines and worth understanding.
- **Polling faster instead of NOTIFY:** that trades latency for constant load on an empty table.
- **Unbounded concurrency plus autoscaling:** scaling takes minutes and an overload spike takes seconds. Shedding covers the gap.
