# 0010. Elasticsearch as a read model, fed by the outbox

- **Status:** Accepted
- **Date:** 2026-10

## Context
Users need to find auctions by words, tolerate typos, and get type-ahead suggestions. PostgreSQL can do basic full-text search, but relevance tuning, fuzzy matching and autocomplete are what search engines are built for. Adding a second datastore raises the usual question: how do the two stay consistent?

## Decision
- **PostgreSQL stays the source of truth.** Elasticsearch is a read model: a denormalised copy (item + auction in one document) shaped for search.
- **Sync through the outbox**, not inside request handlers. Every auction change already emits an event in its own transaction. The `search.Indexer` handles those events by re-reading the auction and upserting it.
- **External versioning** (`version_type=external_gte`, version = `auctions.version`, bumped by a trigger on every UPDATE) means an out-of-order or duplicated event can never roll a document back.
- **Search returns IDs; results are loaded from PostgreSQL** in one `GetMany` query, so prices and statuses shown are exact even if the index lags.
- **PostgreSQL full-text search is the fallback** (generated `tsvector` column plus a GIN index) when Elasticsearch isn't configured or fails.
- **Aliases for zero-downtime reindexing:** reads and writes go through the alias `auctions`; `admin reindex` builds a new index and swaps the alias atomically.

## Alternatives considered
- **Write to Elasticsearch in the handler:** a dual write. A failure after commit leaves the index permanently wrong, and Elasticsearch latency becomes bid latency.
- **Change data capture (Debezium reading the WAL):** robust and generic, but heavy infrastructure for one consumer. The outbox already gives us an ordered, durable change feed.
- **PostgreSQL only:** works (it's the fallback), but no typo tolerance or word-start autocomplete, and search load competes with bidding on the primary database.

## Consequences
- Search is **eventually consistent**: a new auction appears after the next outbox poll (≈2 s).
- Another service to run, but its failure only degrades search quality.
- Mapping changes need a reindex, which is a routine operation.
