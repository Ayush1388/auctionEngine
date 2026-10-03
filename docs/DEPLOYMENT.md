# Deployment

How to run auctionEngine in production on a single Linux machine with Docker Compose, how to ship new versions without downtime, and how to back it up, watch it and scale it.

Everything here is free and open source: Docker, Caddy, PostgreSQL, Redis, Elasticsearch, Kafka, Prometheus, Grafana, Jaeger and Mailpit. No paid service or API is required.

## The deployment

```
                    Internet
                       │  :80 / :443
                ┌──────▼───────┐
                │    Caddy     │  TLS (Let's Encrypt), least-conn load balancing,
                └──┬────────┬──┘  active health checks on /readyz
                   │        │
              ┌────▼──┐  ┌──▼────┐
              │ api-1 │  │ api-2 │  HTTP + WebSockets, outbox worker,
              └───┬───┘  └───┬───┘  lifecycle worker (both instances)
                  │  gRPC    │
                  └────┬─────┘
                ┌──────▼──────┐        ┌────────────┐
                │ biddingsvc  │        │ bidworker  │ ×4 consumers
                └──────┬──────┘        └─────┬──────┘
                       │                     │
   ┌──────────┬────────┴─────┬───────────────┴──┬─────────────┐
   │PostgreSQL│    Redis     │  Elasticsearch   │    Kafka    │
   └──────────┴──────────────┴──────────────────┴─────────────┘

   Prometheus ─ scrapes /metrics on every service (admin ports)
   Grafana    ─ dashboards          Jaeger ─ traces (OTLP)
   Mailpit    ─ SMTP + inbox        backup ─ nightly pg_dump
```

| Container | Image | Public? | Purpose |
|---|---|---|---|
| `caddy` | caddy:2 | **80, 443** | the only public entry point |
| `api-1`, `api-2` | this repo | no | the API (two instances for zero-downtime deploys) |
| `biddingsvc` | this repo | no | bidding over gRPC |
| `bidworker` | this repo | no | Kafka consumers for async bids |
| `migrate` | this repo | no | runs migrations, then exits |
| `postgres` | postgres:18 | no | source of truth |
| `redis` | redis:7 | no | cache, trending, rate limits, WebSocket fan-out |
| `elasticsearch` | 8.15 | no | search read model |
| `kafka` | apache/kafka 3.8 | no | bid commands, event stream |
| `mailpit` | mailpit | 127.0.0.1:8025 | SMTP + inbox (swap for a real provider) |
| `prometheus` | prometheus 2.55 | 127.0.0.1:9090 | metrics + alert rules |
| `grafana` | grafana 11 | 127.0.0.1:3000 | dashboards |
| `jaeger` | jaeger 1.60 | 127.0.0.1:16686 | traces |
| `backup` | postgres:18 | no | nightly dumps to `deploy/backups/` |

## Requirements

- A Linux machine with Docker Engine and the Compose plugin. Elasticsearch and Kafka are Java processes; **4 vCPU and 8 GB RAM** run the whole stack comfortably (it starts in about 4 GB).
- amd64 or arm64: CI publishes images for both, so ARM machines work too.
- For HTTPS: a domain name whose DNS `A` record points at the machine, and ports 80 and 443 open.

## First deploy

```bash
git clone https://github.com/Ayush1388/auctionEngine.git && cd auctionEngine

cp deploy/.env.example deploy/.env
# Fill in the four secrets, each with:  openssl rand -hex 32
#   POSTGRES_PASSWORD, JWT_SECRET, INTERNAL_TOKEN, GRAFANA_ADMIN_PASSWORD
# For HTTPS on a domain:
#   SITE_ADDRESS=auction.example.com
#   PUBLIC_URL=https://auction.example.com

docker compose -f deploy/compose.yml up -d --build --wait
```

`--wait` returns once every container reports healthy. `migrate` runs first, and the API instances only start after it has exited successfully.

Check it:

```bash
curl -s http://localhost/readyz           # {"status":"ok","checks":{"postgres":"ok",...}}
make e2e                                   # the full end-to-end test
```

To run the image CI built instead of building on the server, set `IMAGE=ghcr.io/ayush1388/auctionengine:<commit-sha>` in `deploy/.env` and use `docker compose pull` instead of `--build`. (Make the package public in the GitHub repository settings, or run `docker login ghcr.io` first.)

## Email

The default `SMTP_HOST=mailpit` delivers every email to a local inbox at <http://127.0.0.1:8025>, which is right for testing. For real users, put any SMTP provider's host, port, username and password in `deploy/.env` and remove the `mailpit` service.

## Reaching the private dashboards

Grafana, Prometheus, Jaeger and Mailpit listen on `127.0.0.1` only. From your laptop, tunnel over SSH:

```bash
ssh -L 3000:localhost:3000 -L 9090:localhost:9090 -L 16686:localhost:16686 -L 8025:localhost:8025 you@server
```

Then open <http://localhost:3000> (Grafana, user `admin`), <http://localhost:16686> (Jaeger) and so on. The **Auction Engine** dashboard is provisioned automatically.

## Shipping a new version without downtime

```bash
IMAGE=ghcr.io/ayush1388/auctionengine:<new-sha> ./deploy/rollout.sh
```

What it does:

1. Pulls (or builds) the new image.
2. Runs migrations.
3. Replaces `biddingsvc`, `bidworker`, `api-1`, then `api-2`, **one at a time**, waiting for each to be healthy before the next.

Why no request is dropped: when `api-1` gets SIGTERM it marks itself not ready (`/readyz` → 503), Caddy's 2-second health check stops routing to it, it keeps serving for `DRAIN_DELAY` (5 s), then finishes in-flight requests and exits. Meanwhile `api-2` serves everything. CI proves this on every pull request by running a rollout while a probe sends 10 requests per second and failing on any error.

### Migrations during a rolling deploy

For a few seconds the old and the new code run against the same schema, so every migration must work with both. Change schemas in three releases (**expand → migrate → contract**):

1. *Expand*: add the new column or table; old code ignores it.
2. Deploy code that writes both and reads the new one; backfill.
3. *Contract*: drop the old column in a later release, once nothing reads it.

Never rename or drop something the running version still uses.

### Rolling back

Deploy the previous image the same way: `IMAGE=…:<previous-sha> ./deploy/rollout.sh`. Because migrations are backward compatible, the old code runs on the new schema. Use `go run ./cmd/migrate -down 1` only for a migration that hasn't been relied on yet.

## Backups and restore

The `backup` container runs `pg_dump -Fc` every `BACKUP_INTERVAL_SECONDS` (default daily) into `deploy/backups/` and deletes dumps older than `BACKUP_KEEP_DAYS` (default 7).

- **Copy them off the machine** (e.g. `rsync` to another host or object storage). A backup on the same disk dies with the disk.
- **Test restores**, on a spare machine, regularly:

```bash
./deploy/restore.sh deploy/backups/auction-20261001T020000Z.dump
```

What needs backing up: only PostgreSQL. Redis is a disposable cache (decision 0009), Elasticsearch can be rebuilt with `docker compose exec api-1 /app/admin reindex`, and Kafka holds commands whose outcome is recorded in PostgreSQL.

How much data can be lost: up to one backup interval. For less, enable WAL archiving or use a managed PostgreSQL with point-in-time recovery.

## Operating it

| Task | Command |
|---|---|
| Status | `docker compose -f deploy/compose.yml ps` |
| Logs | `make logs`, or `docker compose -f deploy/compose.yml logs -f api-1` |
| Make someone an admin | `docker compose -f deploy/compose.yml exec api-1 /app/admin promote alice@example.com` |
| Rebuild the search index | `docker compose -f deploy/compose.yml exec api-1 /app/admin reindex` |
| Check the books balance | `GET /v1/admin/reconcile` as an admin |
| See dead-lettered events | `GET /v1/admin/outbox/failed` as an admin |
| CPU profile of a live API | `docker compose -f deploy/compose.yml exec api-1 wget -qO- 'http://localhost:9090/debug/pprof/profile?seconds=30' > cpu.out` |

### Alerts

`deploy/prometheus/alerts.yml` defines the conditions worth waking someone for: an instance down, more than 5% errors, slow p99 latency, slow bids, an outbox backlog or dead letters, Kafka consumer lag, an open circuit breaker and a saturated database pool. Prometheus evaluates them (see *Alerts* in its UI). To be notified, add an Alertmanager container and route alerts to email or chat.

## Security checklist

- Firewall: allow only 22 (SSH, key-only), 80 and 443.
- `deploy/.env` holds every secret, is gitignored and should be readable only by its owner (`chmod 600 deploy/.env`).
- `ENVIRONMENT=production` turns on HSTS.
- Admin listeners (`/metrics`, `/debug/pprof`) are on internal ports; Caddy also refuses those paths.
- `TRUSTED_PROXIES` restricts who may set `X-Forwarded-For`, so clients can't spoof their IP past the rate limiter.
- Containers run as a non-root user.
- Keep the host and images updated: `docker compose pull` and a rollout.

## Scaling

| Pressure | What to do |
|---|---|
| More API traffic | Add `api-3` (copy `api-2`) and list it in the Caddyfile. Instances are stateless; Redis shares rate limits and WebSocket fan-out. |
| More async bids | Raise `BID_WORKERS` or run more `bidworker` containers, up to the partition count (12). |
| Database | Move PostgreSQL to a bigger or managed instance; set `DATABASE_URL`. Add read replicas for search and listings before sharding. |
| One machine is not enough | The same images run on Kubernetes: the probes (`/livez`, `/readyz`), drain delay and config-by-environment are already in place. |
