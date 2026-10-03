# Project notes for Claude

## Frontend work (`frontend/`)

For any UI, page, component or styling task, in every session:

1. Invoke the `frontend-design`, `ui-ux-pro-max` and `taste-skill` skills (in `.claude/skills/`) before writing code, and follow them.
2. Verify the result with Playwright: load http://localhost:5173, take desktop (1440x900) and mobile (390x844) screenshots, check the console for errors, and look at the screenshots before reporting done.
3. If `frontend/DESIGN.md` exists, treat it as the design-system source of truth.

Design direction: vintage-car auction brand. Cream `#F8FAED`, red `#AD221D`, black `#000101`; Koulen display, DM Sans body, DM Mono labels. Reference: the Stradex mockups the user supplied (stacked vertical nav, hamburger top right, giant right-aligned wordmark, awards list, arrows/season/timer/Join now row, full-bleed photo with thumbnails).

Run the frontend locally with `cd frontend && python3 -m http.server 5173`.

Playwright lives outside the repo (installed in a scratchpad); reinstall with `npm i playwright && npx playwright install chromium` if needed. Linux needs `sudo npx playwright install-deps chromium` once.
