# 0014. Observability: Prometheus metrics, OpenTelemetry traces, health probes

- **Status:** Accepted
- **Date:** 2026-10

## Context
By v0.9 a single bid can touch an HTTP gateway, a gRPC service, PostgreSQL, the outbox, Kafka, a bid worker, Redis and Elasticsearch. Logs (with request IDs) tell us what happened in *one* process. They can't answer "is the system healthy right now?", "where did the 800 ms go?" or "should this pod get traffic?". We need all three before calling anything production-ready.

## Decision
- **Metrics (Prometheus, `internal/metrics`).** RED metrics for HTTP and gRPC (rate, errors, duration histograms); domain metrics (`bids_total{result}`, `bid_duration_seconds{strategy}`, outbox processed/retried/dead-lettered, async bid outcomes, search backend and fallback, 429s by rule); and state read at scrape time (DB pool, outbox backlog, WebSocket connections and drops, cache hits/misses, Kafka consumer lag). A private registry; every label is drawn from a bounded set (route *pattern*, never raw path; outcome names, never error text).
- **Route labels through a context holder (`httpx.Tagged`/`httpx.Route`).** `ServeMux` sets `r.Pattern` only on its own copy of the request, so outer middleware can't see it. A pointer stored in the context is shared by every copy.
- **Traces (OpenTelemetry, `internal/telemetry`).** W3C `traceparent` propagation across every hop: HTTP headers, gRPC metadata, the outbox row (`outbox_events.trace_context`, migration 000015), and Kafka record headers. SQL statements become spans through a pgx `QueryTracer` that records statement text only (never bind arguments) and only inside an existing trace. Export over OTLP/HTTP with a batching processor and a parent-based ratio sampler. Off (no-op) unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set. `trace_id` is added to every log line.
- **Probes (`internal/health`).** `/livez` never checks dependencies (restarting a pod doesn't fix Postgres). `/readyz` fails only on *critical* dependencies (Postgres; for the bid worker, Kafka too). Optional ones are reported but keep the pod in rotation. Each check has a 2 s timeout.
- **Draining.** On SIGTERM, `/readyz` turns 503 at once, the process keeps serving for `DRAIN_DELAY`, and only then is the listener closed. Deploys drop no requests.
- **Admin listener (`ADMIN_ADDR`).** `/metrics` and pprof on a separate internal port, never routed through the public load balancer.

## Consequences
- One trace shows a bid end to end, including the asynchronous hops. Tests assert this (`TestOneTraceFromHTTPThroughGRPCToSQL`, `TestTraceSurvivesOutboxAndKafka`), and they fail if propagation is removed at any hop.
- A few microseconds per request for metrics. Spans are nearly free when tracing is off.
- Every new label must be checked for cardinality. An unbounded label is the classic way to take Prometheus down.

## Alternatives considered
- **otelhttp/otelgrpc contrib instrumentation:** less code, but it hides exactly what we want to learn (carriers, span kinds, propagation). The hand-written versions are about 150 lines.
- **Metrics on the public port:** simpler, but exposes internals and pprof.
- **Readiness that fails on any dependency:** turns a Redis blip into a full outage.
