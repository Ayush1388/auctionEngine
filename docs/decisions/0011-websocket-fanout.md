# 0011. WebSocket live updates with Redis pub/sub fan-out

- **Status:** Accepted
- **Date:** 2026-10

## Context
Watchers of an auction should see new bids immediately. Polling `GET /v1/auctions/{id}` every second from thousands of browsers wastes database reads and still lags. The harder problem: the API runs as several instances behind a load balancer, and the outbox hands each event to **one** instance, while the watchers of one auction are connected to **all** of them.

## Decision
- **WebSockets** (`GET /v1/ws`, `coder/websocket`): one long-lived connection per browser, with subscribe/unsubscribe per auction.
- **Outbox → snapshot → fan-out:** the instance that handles an auction event reloads the auction and publishes a full snapshot to a Redis pub/sub channel; every instance's subscriber delivers it to its local room. Without Redis, a local publisher serves the single instance.
- **Snapshots, not deltas, with a version:** clients apply only higher versions, so out-of-order or duplicate messages are harmless and nothing needs replaying.
- **Per-connection writer goroutine** with a bounded buffer. If the buffer is full, the client is disconnected (1008) so one slow phone can't stall a room or exhaust memory.
- **Heartbeats** (ping every 25 s, 10 s to answer) detect half-open connections.
- **Limits:** origin allow-list (blocks cross-site WebSocket hijacking), 4 KB inbound frames, 20 subscriptions per connection, a per-instance connection cap (1013 when full).
- **Shutdown:** `http.Server.Shutdown` ignores hijacked connections, so the hub closes them with 1001 via `RegisterOnShutdown`.

## Alternatives considered
- **Server-Sent Events:** simpler (plain HTTP, auto-reconnect), one-way only. Viable here; WebSockets chosen to support subscribe commands on one connection and to learn the upgrade/heartbeat mechanics.
- **PostgreSQL LISTEN/NOTIFY for fan-out:** no extra service, but each listener holds a dedicated connection and payloads are capped at 8 KB; Redis is already deployed.
- **Kafka for fan-out:** durable but heavyweight for ephemeral UI updates (v0.8 uses Kafka for the bid pipeline, where durability matters).
- **Sticky sessions per auction:** avoids fan-out, but a viewer watching many auctions breaks the routing, and load balances badly.

## Consequences
- Updates are best-effort: a message published while an instance is disconnected from Redis is lost, and the client catches up on its next message or reconnect. Correctness never depends on them.
- Memory per instance is bounded: connections × buffer size.
