# 0003. `FOR UPDATE SKIP LOCKED` for background workers

- **Status:** Accepted
- **Date:** 2026-09

## Context
Background work (the outbox today, auction lifecycle next) must be safe when several API instances run the same worker. Two workers must never process the same row, and a crashed worker must not strand its rows forever.

## Decision
Workers claim a batch with `SELECT ... FOR UPDATE SKIP LOCKED` inside an `UPDATE ... RETURNING`, set `locked_at`, and treat a lock older than a lease timeout (5 minutes) as abandoned. A partial index on pending rows keeps polling cheap.

## Alternatives considered
- **Plain `FOR UPDATE`** – workers queue behind each other's locks instead of taking different rows.
- **A single worker** – simple, but a single point of failure and no horizontal scaling.
- **External queue** – more infrastructure for a problem Postgres already solves at this scale.

## Consequences
- Polling adds a small, steady load on the database.
- Work must be idempotent, because a lease can expire while a slow worker is still running.
