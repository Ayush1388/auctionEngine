# The frontend

A vintage-car auction site (Marque) built on the engine. It is plain HTML, CSS and ES modules: no build step, no framework. Its job is to make the backend visible: every page reads from the real API, updates live over the WebSocket, and can show what happened behind a bid.

```bash
cd frontend && python3 -m http.server 5173      # http://localhost:5173
```

## Three ways to run it

| Mode | What it needs | What you get |
|---|---|---|
| **Real backend** | `make infra && make run` (API on :4000), then `go run ./cmd/demobots seed` | Every number is measured on PostgreSQL, the outbox and the WebSocket. The status pill says "Live backend". |
| **Plus the demo bots** | `DEMO_BOTS_ENABLED=true` on the API (add `DEMO_SEED=true` to load the cars at start-up) | The bots are built into the API. The switch in the badge turns rival bidders on and off, and Stress it fires hundreds of bidders at one lot. No second server, no `RATE_LIMITS=off`. |
| **No backend** | nothing | The site runs on a built-in **demo engine** that follows the same rules, in the browser. The pill says "Demo engine" and every timing is labelled simulated. |

The page tries the real API first (`http://localhost:4000`, 1.5 s) and falls back to the demo engine. Force a choice with `?engine=sim`, `?engine=live` or `?engine=auto`; set other endpoints with `?api=` and `?jaeger=`. The choice is remembered.

Demo accounts (real backend after `demobots seed`, and the demo engine): `demo@marque.test`, `collector@marque.test`, `admin@marque.test`, password `marque-demo-password`, each with $2,000,000. Sign in as two different people in two windows to see two bidders on one lot.

## Start here: the backend tour

`tour.html` is the entry point for explaining the system. It lays out eight stages in the order a bid meets them (the request, the decision, the money, the events, search, resilience, observability, other formats). Each stage says what happens, what it is built with and why, lights its parts of the diagram, and opens a live demo page. Every demo page carries the same strip at the top (stage number, one sentence, tech chips, previous and next), so they read as one walkthrough. Stage data lives in `app/tour.js`.

## What each page shows

| Page | Backed by | What it demonstrates |
|---|---|---|
| Home, event | `GET /v1/auctions`, WebSocket | Live prices and clocks on every card; the Bid button places a real bid at the minimum |
| **Lot** | bids, wallet, WebSocket | A bid in one window appears in the others in milliseconds. Two people bidding in the same instant: exactly one wins, the other is told the new minimum and how long it waited behind the winner. Held money is shown and released the moment you are outbid. A bid in the last two minutes extends the close. Double-clicking Bid places one bid (same `Idempotency-Key`). |
| **Behind the bid** | `Server-Timing`, WebSocket timestamps | The journey of one bid: network and gateway, the row lock, the rules, the write, the commit, the outbox to fan-out, the click to the live update. Each number says where it comes from. |
| **Stress it** | `cmd/demobots` or the demo engine | 200 bidders read the same price and bid at once for several rounds. The page then checks, from the outside, that the history is strictly increasing, the counts match, one bidder leads and nothing failed. |
| Sign in, create, activate | `/v1/users/*`, `/v1/auth/*` | Argon2id accounts, activation tokens, JWT access tokens and rotating refresh tokens (per tab, so two windows can be two people) |
| Wallet | `/v1/wallet`, `/deposits`, `/ledger` | Available and held money, idempotent deposits, and the double-entry statement |
| My bids | the ledger, bid history | Leading, outbid, won, lost, saved; worked out from the ledger, which records every hold, release and sale |
| Sell a car | `POST /v1/auctions`, `/cancel` | A listing form that maps the server's field errors, a live preview, my listings with cancel |
| Search, trending | `/search`, `/trending` | Typo-tolerant search that says which backend answered (Elasticsearch or the PostgreSQL fallback) |
| Alerts | WebSocket | "You were outbid", "you won", "closed" |
| Status | `/livez`, `/readyz`, `/v1/admin/metrics` | Probes for everyone; bids per second, p95 latency, queue depth and breakers for operators |
| Operator console | `/v1/admin/reconcile`, `/outbox/failed` | "Do the books balance?" and retrying dead-lettered events |
| **Maximum bid** (lot page) | `PUT/GET/DELETE /v1/auctions/{id}/proxy-bid` | Proxy bidding: set the most you will pay and the engine raises your bid for you, up to it. Bids it places are marked "auto". |
| **Trust page** | `/v1/admin/reconcile`, bids, wallet | A live "do the books balance" counter, a double-spend attack (many bids, one wallet, checked by the page), a retry with one Idempotency-Key, and the rate-limit bucket |
| **Chaos lab** | `/v1/admin/chaos` (needs `CHAOS_ENABLED=true`) | Break Redis, Kafka, Elasticsearch, the bidding service or PostgreSQL, keep bidding, and watch breakers open and search fall back |
| **Architecture map** | probes, metrics, WebSocket | The services a bid touches, with a pulse for every real event, per-node health, and the Kafka lane each lot is pinned to |
| Race theater | bids, `Server-Timing`, PERFORMANCE.md | Several bids on one price, the lock queue, and pessimistic against optimistic locking on the measured numbers |
| Search lab | `/search` | The same query through Elasticsearch and the PostgreSQL fallback, side by side |
| Auction formats | demo only | Dutch and sealed-bid (first and second price) on a small ledger with the engine's money rules |
| System pulse | `/livez`, `/readyz` | The demo/live badge on every page shows health and round-trip time, and links to the map |

## How a bid shows up on screen

```mermaid
sequenceDiagram
    autonumber
    participant B as Browser (window A)
    participant G as API gateway
    participant S as Bidding service
    participant P as PostgreSQL
    participant O as Outbox worker
    participant R as Redis pub/sub
    participant W as Browser (window B)

    B->>G: POST /bids  Idempotency-Key
    G->>S: PlaceBid (in-process or gRPC)
    S->>P: BEGIN, lock auction, decide
    S->>P: bid, hold, ledger journals, outbox event
    S->>P: COMMIT
    S-->>G: result and phase timings
    G-->>B: 201 and Server-Timing
    P-->>O: event (LISTEN/NOTIFY)
    O->>R: publish snapshot
    R-->>W: WebSocket auction.updated
    R-->>B: WebSocket auction.updated
```

Kafka is not in this picture because it is only on the optional asynchronous path (`Prefer: respond-async`). A synchronous bid skips it, and the panel says so instead of drawing a hop that did not happen.

## What the backend gained for this

All additive; existing clients are unaffected.

| Change | Why |
|---|---|
| `Server-Timing` on `POST /v1/auctions/{id}/bids`: `lock`, `decide`, `write`, `commit`, `bidding` (also on refused bids) | The panel's timings are measured by the server. A refused bid's `lock` is how long it waited behind the winner. |
| `X-Trace-ID` response header when tracing is on | Link a bid to its trace |
| WebSocket `auction.updated` gains `bid` (id, bidder, previous bidder, amount) and `timing` (placed and sent, both from the API clock) | A client can match its own bid, know it was outbid, and see the outbox delay without clock sync |
| `GET /v1/admin/metrics` | The status page reads the same series Prometheus scrapes, behind the admin role |
| CORS exposes the headers above | A browser can only read exposed headers |
| `bids.created_at` is `clock_timestamp()` | A bug the Stress it read-back found: see [decision 0006](decisions/0006-bid-concurrency.md) |

`internal/demobots` is dev tooling built into the API (only when `DEMO_BOTS_ENABLED=true`): `Seed` writes the catalogue and demo accounts through the real services, so every seeded bid has a real hold and ledger entry (`go run ./cmd/demobots seed` does the same from the command line, and `DEMO_SEED=true` at start-up); the ambient rivals and Stress it bid through the same bidder the HTTP handlers use, so holds, the ledger, the outbox, the WebSocket and the chaos faults are all real. They skip the HTTP layer and the rate limiter, which is why no `RATE_LIMITS=off` is needed. Endpoints: `GET/PUT /v1/demo/bots`, `POST /v1/demo/stress`, `GET /v1/demo/stress/{id}/events`, `POST /v1/demo/seed` (operator).

## How it is organised

```
frontend/
  *.html, *.js, *.css      the original site: home, event, lot pages
  app/
    backend.js             picks the real API or the demo engine
    live.js  api.js  feed.js  session.js   the real API: REST, refresh rotation, WebSocket
    sim-engine.js          the demo engine's rules (pure, tested in Node)
    sim.js                 the demo engine as a backend: host window, shared across windows
    bidding.js             one lot: loading, live updates, placing a bid, explaining the outcome
    trace.js  chart.js     Behind the bid, charts
    shell.js  cards.js     header, wallet chip, alerts, engine badge; live cards
    catalog.json           the 12 cars: the single source for the site, the demo engine and the Go seeder
    pages/*.js  *.css      one script per page
  tests/                   node --test
```

Both backends implement one interface (`app/live.js` and `app/sim.js`), so a page never knows which it is talking to.

**The demo engine.** `sim-engine.js` is an in-memory auction house with the backend's rules: minimum bid, one leader, held and released money, double-entry journals, anti-sniping, idempotency, settlement, refresh-token rotation and reuse detection. `sim.js` adds what a server has: all windows on a device share one engine (the window holding a Web Lock hosts it, the others talk to it over a BroadcastChannel, and another window takes over if the host closes), simulated network and outbox delay, rival bidders, and the stress runner. `tests/sim-engine.test.mjs` checks the rules, including 500 random bids that must keep every ledger invariant.

**Listings and photos.** The API's item has a name, type and description, so a car's year, make, photo and specs travel in a metadata block at the end of the description (`[[marque:{...}]]`, see `app/model.js`). An uploaded photo stays in the uploader's browser; everyone else sees the house photo chosen with it.

## Hosting a demo

Publish the `frontend/` folder to any static host (Netlify, Vercel, GitHub Pages). With no API reachable it runs on the demo engine, so the link works for anyone, with no backend. To point a hosted copy at a deployed API, add its origin to `CORS_ALLOWED_ORIGINS` and open the page once with `?api=https://your-api.example`.

## Recording the demo

`docs/media/demo.mp4` is two real windows (Demo bidder and Collector) against the real backend: a simultaneous bid, Behind the bid, then Stress it. To make your own, run the backend and bots as above, then record two browser windows side by side; the sequence is: both click Place bid in the same instant, the loser rebids, open Behind the bid, run Stress it.

## The under-the-hood pages

These move the backend into the main experience instead of hiding it behind admin screens.

**Chaos lab and the real fault switch.** `internal/chaos` is an in-memory switchboard, off unless the API starts with `CHAOS_ENABLED=true`. Operators drive it with `GET/PUT /v1/admin/chaos` and `POST /v1/admin/chaos/reset`. The hooks: a go-redis hook fails every Redis command; the Elasticsearch guard, the Kafka relay and the readiness probes fail on request; a breaker-guarded bidder fails bids with `ErrUnavailable`; a pgx tracer delays every statement. While off, each hook is one atomic load. Never enable it on a production instance.

```bash
CHAOS_ENABLED=true RATE_LIMITS=off CORS_ALLOWED_ORIGINS=http://localhost:5173 make run
```

On the hosted demo (no API) the same page drives the demo engine's modelled faults and says so in a banner: breakers open after five failures and half-open after ten seconds, like the Go ones. Timings there are simulated.

**Proxy bidding.** Migration 17 adds `proxy_bids` and `bids.auto`. A manual bid or a new maximum is answered inside the same transaction, under the same auction row lock, by `nextAutoBid` (a pure function, tested without a database; ported to `app/proxy-rules.js` for the demo engine). The higher maximum wins and pays one increment over the lower one; a tie goes to the earlier instruction; only the visible bid is held, and a maximum is capped by the funds available when it is needed. Wallets of every instruction owner are locked together in user-ID order. The proxy endpoints run in the gateway process against the same database, even when bidding itself is a separate gRPC service.

**Kafka lanes.** `app/partition.js` reproduces Kafka's murmur2 partitioner (checked against Kafka's own test vectors), so the page shows the lane each lot really maps to. Lag per partition comes from `auction_kafka_consumer_lag`.

**Trace waterfall.** The Behind the bid panel draws a timeline from the round trip, `Server-Timing` and the event timestamps. With a Jaeger address (`?jaeger=http://localhost:16686`) and a trace ID it fetches and draws the real spans instead (Jaeger must allow the page's origin).

## Limits worth knowing

- What the hosted demo shows is simulated, and labelled as such. A free static host cannot run Kafka, Elasticsearch, PostgreSQL and Redis, so the real chaos lab is for your own machine (and a recording).
- Dutch and sealed-bid auctions are demo-only; the API sells English auctions.
- Proxy bidding is covered by Go integration tests (`internal/bidding/proxy_test.go`) that need `TEST_DATABASE_URL`; they skip without it.
- Type-ahead (`/suggest`) is Elasticsearch-only on the API. Without Elasticsearch the page falls back to matching the open lots' titles in the browser.
- Search without Elasticsearch has no typo tolerance; the page says which backend answered.
- Activation links in emails point at the API (`/v1/users/activate?token=`). The Activate tab takes the token from that link.
- Tokens live in `sessionStorage`: they are readable by script on the page. That is a known trade-off for a token-in-header API; the API sets a strict CSP on its own responses.
- `Server-Timing` splits a bid into lock, rules, write and commit when bidding runs in the API process. With the bidding service split out over gRPC the gateway reports the whole call as one step.

## Tests

```bash
node --test frontend/tests/sim-engine.test.mjs        # the demo engine's rules
go test ./internal/bidding ./internal/handlers ./internal/realtime ./internal/metrics ./internal/middleware
```
