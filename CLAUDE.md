# Project notes for Claude

## Frontend work (`frontend/`)

For any UI, page, component or styling task, in every session:

1. Invoke the `frontend-design`, `ui-ux-pro-max` and `taste-skill` skills (in `.claude/skills/`) before writing code, and follow them.
2. Verify the result with Playwright: load http://localhost:5173, take desktop (1440x900) and mobile (390x844) screenshots, check the console for errors, and look at the screenshots before reporting done.
3. If `frontend/DESIGN.md` exists, treat it as the design-system source of truth.

Design direction: vintage-car auction brand. Cream `#F8FAED`, red `#AD221D`, black `#000101`; Koulen display, DM Sans body, DM Mono labels. Reference: the Stradex mockups the user supplied (stacked vertical nav, hamburger top right, giant right-aligned wordmark, awards list, arrows/season/timer/Join now row, full-bleed photo with thumbnails).

Run the frontend locally with `cd frontend && python3 -m http.server 5173`.

Playwright lives outside the repo (installed in a scratchpad); reinstall with `npm i playwright && npx playwright install chromium` if needed. Linux needs `sudo npx playwright install-deps chromium` once.

## The app layer (`frontend/app/`)

The pages talk to a backend through `app/backend.js`: the real API when it answers (`localhost:4000`), otherwise the built-in demo engine (`app/sim.js`, rules in `app/sim-engine.js`). Both implement the same interface, so a change to the interface needs both (`live.js` and `sim.js`). Full guide: `docs/FRONTEND.md`.

- New pages: ES modules under `app/pages/`, header and footer from `app/shell.js` (`initShell({ bar: true, footer: true })`), styles from `app/app.css` and `app/pages.css`. Do not use a `.num` class (the home page owns it); use `.tnum`.
- Everything from the network goes through the `html` tagged template in `app/util.js`, which escapes. Never put API text into `innerHTML` any other way.
- Test both engines: `?engine=sim` and `?engine=live`. Live needs Postgres and Redis, `make migrate`, the API with `RATE_LIMITS=off` and `CORS_ALLOWED_ORIGINS=http://localhost:5173`, `go run ./cmd/demobots seed`, and `go run ./cmd/demobots serve` for Stress it. Demo accounts: `demo@marque.test`, `collector@marque.test`, `admin@marque.test`, password `marque-demo-password`.
- The demo engine's rules have tests: `node --test frontend/tests/sim-engine.test.mjs`. Keep them green when changing `sim-engine.js`.
- No em dashes in visible copy (use a hyphen or restructure the sentence).
- Every text file is LF (enforced by `.gitattributes`). CRLF breaks `gofmt` and the `deploy/*.sh` scripts in CI.
