# 0013. Extract bidding into a gRPC service behind the gateway

- **Status:** Accepted
- **Date:** 2026-10

## Context
Bidding has different scaling and reliability needs from the rest of the API: it is the hot path during an auction's last minute, it holds row locks, and its correctness rules (money, ordering) are the core of the system. We want to deploy, scale and protect it separately, and to learn what a service boundary costs.

## Decision
- **`cmd/biddingsvc`** serves `auctionengine.bidding.v1.BiddingService` (`api/proto/bidding/v1/bidding.proto`): `PlaceBid`, `ListBids` (unary) and `WatchAuction` (server streaming).
- **The gateway (`cmd/api`) keeps the public HTTP API.** Handlers depend on the `handlers.Bidder` interface; `bidding.Service` (in-process) and `grpcsvc.Client` (remote) both implement it. `BIDDING_GRPC_ADDR` picks one. No handler changed.
- **Errors keep their meaning across the wire:** domain errors become gRPC codes with `ErrorInfo`/`BadRequest` details, and the client turns them back into the same Go errors. Unavailable or timed-out service → `bidding.ErrUnavailable` → HTTP 503 with `Retry-After`.
- **Deadlines everywhere:** the client sets 3 s by default, the server applies 5 s if none arrived, and the context reaches pgx, so a caller giving up cancels the database query (tested with a held row lock).
- **Retries only where safe:** a gRPC retry policy (UNAVAILABLE, ABORTED; 3 attempts; exponential backoff) is safe because the client always sends an idempotency key, generated per HTTP request when the user didn't supply one.
- **Service-to-service auth:** a shared `INTERNAL_TOKEN` in metadata, compared in constant time; health checks are exempt for probes. The request ID travels in `x-request-id` metadata, so logs from both services correlate.
- **Interceptors** for recovery, logging, auth and default deadlines. Plus the standard health service, reflection (for `grpcurl`), and `GracefulStop` on shutdown.

## What is NOT split yet, deliberately
The bidding service still uses the **same PostgreSQL database** as the gateway, and bidding writes columns of `auctions` (`current_bid`, `bid_count`, `version`) in the same transaction as wallet reservations. Splitting the database would turn that one ACID transaction into a distributed one (a saga with compensations). The honest status:

| Concern | Owner today | Path to full independence |
|---|---|---|
| bids, wallets, ledger, reservations | bidding service | move to its own database |
| auction catalogue (items, times, status) | gateway | stays |
| live bidding state on `auctions` | shared table | move into bidding's DB; publish `bid.placed` so the catalogue reads a projection |
| settlement on `auction.completed` | gateway's outbox worker | bidding service consumes `auction-events` from Kafka (v0.8) |

## Alternatives considered
- **REST between services:** simpler tooling, but no schema-enforced contract, no streaming, and hand-rolled error mapping.
- **Split everything at once (database per service):** the right end state, but a distributed transaction for every bid before we've measured the need.

## Consequences
- One network hop per bid (≈ sub-millisecond on a LAN) and a new failure mode: the service being down → 503, tested.
- Contract changes must stay backward compatible (protobuf field numbers are forever).
