# Project notes for Claude

## Frontend work (`frontend/`)

For any UI, page, component or styling task, in every session:

1. Invoke the `frontend-design`, `ui-ux-pro-max` and `taste-skill` skills (in `.claude/skills/`) before writing code, and follow them.
2. Verify the result with Playwright: load http://localhost:5173, take desktop (1440x900) and mobile (390x844) screenshots, check the console for errors, and look at the screenshots before reporting done.
3. If `frontend/DESIGN.md` exists, treat it as the design-system source of truth.

Design direction: vintage-car auction brand. Cream `#F8FAED`, red `#AD221D`, black `#000101`; Koulen display, DM Sans body, DM Mono labels. Reference: the Stradex mockups the user supplied (stacked vertical nav, hamburger top right, giant right-aligned wordmark, awards list, arrows/season/timer/Join now row, full-bleed photo with thumbnails).

Run the frontend locally with `cd frontend && npm install && npm run dev` (React + Vite). Pages are in `frontend/src/pages`, shared pieces in `src/components`, `src/hooks`, `src/lib`; the framework-free core stays in `frontend/app/` and is imported by `src/`. `npm run build` must pass, and `npm test` keeps the engine rules green.

Playwright lives outside the repo (installed in a scratchpad); reinstall with `npm i playwright && npx playwright install chromium` if needed. Linux needs `sudo npx playwright install-deps chromium` once.

## The app layer (`frontend/app/`)

The pages talk to a backend through `app/backend.js`: the real API when it answers (`localhost:4000`), otherwise the built-in demo engine (`app/sim.js`, rules in `app/sim-engine.js`). Both implement the same interface, so a change to the interface needs both (`live.js` and `sim.js`). Full guide: `docs/FRONTEND.md`.

- New pages: a React component in `src/pages/` plus a lazy `<Route>` in `src/App.jsx`. Header and footer come from `components/Layout.jsx`; styles are in `src/styles/` (`app.css`, `pages.css`, `react.css`). Do not use a `.num` class (the home page owns it); use `.tnum`.
- Render API text as React children (JSX escapes it). Never use `dangerouslySetInnerHTML` with anything from the network.
- The interface also has `setProxy/getProxy/cancelProxy`, `chaos.get/set/reset`, `search(q, {force})` and, on the demo engine only, `trust.attacker(budget)`. The under-the-hood pages (Trust, Chaos, System, Race, SearchLab, Formats, Tour) share `src/lib/lab.js`, `src/hooks/system.js`, `src/lib/archmap.js`, `components/TracePanel.jsx` and `styles/lab.css`.
- Test both engines: `?engine=sim` and `?engine=live`. Live needs Postgres and Redis, `make migrate`, and the API with `CORS_ALLOWED_ORIGINS=http://localhost:5173 DEMO_BOTS_ENABLED=true DEMO_SEED=true CHAOS_ENABLED=true` (the demo cars, the rival bots and Stress it are built in: there is no second server; `RATE_LIMITS=off` is only needed to see the wallet guard without the rate limiter shedding a burst first). Demo accounts: `demo@marque.test`, `collector@marque.test`, `admin@marque.test`, password `marque-demo-password`.
- The demo engine's rules have tests: `node --test frontend/tests/sim-engine.test.mjs`. Keep them green when changing `sim-engine.js`.
- No em dashes in visible copy (use a hyphen or restructure the sentence).
- Every text file is LF (enforced by `.gitattributes`). CRLF breaks `gofmt` and the `deploy/*.sh` scripts in CI.
