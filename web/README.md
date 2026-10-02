# Aurelian web

Frontend for auctionEngine. Design and scope are in [`../plan.md`](../plan.md).
Only the Discover page (`/`) is built so far; other routes show a
"not built yet" page.

## Run

```bash
npm install
cp .env.example .env
npm run dev          # http://localhost:5173
```

The app reads the API at `VITE_API_URL` (default `http://localhost:4000`).
Either run the real API (`make run` in the repo root, with
`CORS_ALLOWED_ORIGINS=http://localhost:5173`), or the mock:

```bash
npm run mock         # mock API + WebSocket on :4000, seeded lots, simulated bids
```

The mock serves only what Discover reads (`GET /v1/auctions`,
`/v1/auctions/trending`, `/v1/auctions/{id}`, `/v1/ws`) in the shapes of
`api/openapi.json`. See the header of `mock/server.mjs` for switches
(`MOCK_QUIET`, `MOCK_EMPTY`, `MOCK_NO_LIVE`, `MOCK_FAIL`, `MOCK_DELAY`).

## Check

```bash
npm run typecheck
npm run build
npm run inspect      # Playwright: screenshots at four viewports + console, network and overflow report
```

`npm run inspect` needs the dev server running and a Chromium for Playwright
(`npx playwright install chromium`, or set `PW_CHROMIUM` to an existing binary).

## Photos

There is no image storage in the API yet. See `public/lots/README.md` for the
interim manifest. Until photos are added, lots show a typographic placeholder.
