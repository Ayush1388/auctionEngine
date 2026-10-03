# 0004. Hand-written migration runner

- **Status:** Accepted
- **Date:** 2026-09

## Context
The schema needs versioned, repeatable changes. This project is also a learning exercise, so understanding how migrations work matters.

## Decision
A small runner in `internal/migration` applies numbered `up`/`down` SQL files from `migrations/` and records applied versions in `schema_migrations`.

## Alternatives considered
- **golang-migrate / goose** – production-ready and the likely choice in a team, but they hide the mechanics this project set out to learn.

## Consequences
- Edge cases (locking against concurrent runs, dirty state after a failed migration) are ours to handle.
- Swapping to golang-migrate later is straightforward because the file format is similar.
