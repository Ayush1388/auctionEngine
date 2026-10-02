# Frontend Plan — Auction Engine

Status: design plan only. No frontend code exists yet. This document is the single source of design decisions for the implementing session.

Read order for the implementer: §0 (facts and caveats) → §1 (principles) → §3 (system) → §4 (pages) → §5–9 → §12 (backend gaps) → §13 (priority).

---

## 0. Ground truth and caveats

### 0.1 What I inspected
- Repo is a Go backend only (v1.0). **No frontend, no design system, no `web/` directory.** README roadmap says "Next: web frontend".
- Contracts read: `api/openapi.json` (OpenAPI 3.1), `internal/realtime/*` (WebSocket protocol), `internal/bidding/*` (bid rules), `internal/handlers/bid.go` (error mapping), migrations, `.env.example`, `docs/examples/live-auction.html`, ADRs listed in README.
- `.env.example` sets `CORS_ALLOWED_ORIGINS=http://localhost:5173`, so a Vite dev server is the expected frontend host.

### 0.2 Skills
- `/frontend-design` — loaded and applied (distinct identity, no template defaults, one memorable element, restraint, plain-language copy, build-quality floor).
- `/ui-ux-pro-max` — **not available in this session** (`Unknown skill`, no file on disk). I did not invent its guidance. Compensation: a standard UX checklist (Nielsen heuristics, WCAG 2.2 AA, 44px touch targets, form/error patterns) is used throughout. If the skill is installed later, re-run it against §4–§9 and reconcile.
- `design:accessibility-review` and `design:ux-copy` exist as plugin skills; use them for a post-implementation audit (§13, phase 7).

### 0.3 The backend is thinner than the references
The reference sites show photos, makes/models, specs, reserve status, locations and watchlists. **The current API has none of these.**

| Reference feature | Backend today | Plan treatment |
|---|---|---|
| Vehicle photos | None. `Item = {id, name, type, description}` | Design for photos fully; ship placeholders; blocked on **B-1** |
| Year/make/model/specs | None; free-text `name`, `type` (≤50), `description` (≤5000) | Interim convention in one adapter (§10.3); blocked on **B-2** |
| Reserve met / not met | No reserve concept. Any bid ≥ starting price wins at close | Show "No reserve" (true under current rules; confirm in §12 Q1). Component supports future states |
| Location, transmission, fuel, body, make filters | None | Filter UI is config-driven; shows only what the API supports. **B-3** |
| Sort by ending soon / price / most bids | List is newest-first only; `trending` = most bids last hour | Interim client sort over ≤100 active auctions; **B-4** |
| Watchlist | None | Device-local interim, labelled honestly; **B-5** |
| "My bids" | No endpoint | Derive from wallet ledger (§4.8); **B-6** |
| Bidder names | History exposes only `user_id` | Anonymise as "Bidder 1/2/3" per auction, "You" for self |
| Seller profile | Only `owner_id` | "Private seller" card; **B-7** |
| Proxy / max bidding | **Does not exist.** A bid is a plain bid; you pay what you bid | **Do not design "your max bid".** See §4.4 |
| Forgot password | No endpoint (only resend-activation) | Do not ship UI; **B-8** |
| Total result counts, page numbers | Keyset cursors only | "Load more", no page numbers |

**Rule: never fake data the backend cannot support.** Fixtures for design review (a `/_dev/gallery` route, dev builds only) must be clearly labelled fixtures.

---

## 1. Design principles

1. **CAR = emotion, UI = clarity, AUCTION = urgency + trust.** Photography carries feeling. Interface chrome stays quiet. Auction mechanics are exact and unambiguous.
2. **Content first.** The first viewport of every page shows auctions or the bid, not marketing. No hero taller than ~40% of a laptop viewport on Home.
3. **One memorable element: the Bid Slip.** The bid panel is the signature object (§3.8). Everything else is disciplined.
4. **Money UI is mood-invariant.** Vintage/modern atmosphere changes the photo stage only. Prices, countdowns, bid forms and states look identical on every auction, so users learn them once and trust them.
5. **Server truth, never optimistic money.** The current bid, leader and end time change only when the server says so (REST response or WebSocket snapshot). Pending state is shown as pending.
6. **Every state is designed** (§7). The happy path is the minority case in an auction.
7. **Capability-driven UI.** Features the API lacks are hidden or labelled, not faked; they appear automatically when the backend adds them (§10.5).
8. **Decorative is out.** No glass, no gradients as decoration, no blur panels, no ambient animation.
9. **Never alter vehicle photos.** No CSS filters, tints, overlays or watermarks on listing photos. Buyers are judging condition with real money.

---

## 2. Reference analysis (principles extracted, nothing cloned)

### Image 1 — Stradex laptop mockup (vintage, light)
- **Observed:** ivory field, oversized condensed red wordmark, three-colour discipline (ivory / red / black), tiny red micro-labels, vertical thumbnail selector beside a wide warm photo, thin outlined pill buttons, small countdown.
- **Take:** three-colour restraint; condensed display type as the identity; thumbnail strip as a selector; warm photography against a quiet field.
- **Reject:** ~10px text (fails readability), giant wordmark (pushes content down), pill buttons everywhere.

### Image 2 — Koulen type and palette card (vintage, brand)
- **Observed:** heavy condensed capitals; ivory `#F8FAED`, red `#AD221D`, black `#000101`; faded, warm, saturated-orange desert road photograph.
- **Take:** that exact warm-ivory / oxblood / black triad is the classic-car foundation; condensed display for titles and numbers; photography grade is warm and slightly faded (we only *source* photos like this for marketing slots, never alter listing photos).
- **Reject:** frosted-glass card over the photo.

### Image 3 — Dark event page and docket grid (modern auction house)
- **Observed:** near-black UI; white heavy sans title; outlined secondary buttons; icon tab row (Event Info / Dockets / Buy Tickets…); grid cards with photo on top, thin red hairline, "Lot #693" right-aligned, event name small, title below; toolbar with "Show 24 per page", search, "All filters" outlined button, "Sort by"; category tabs; mobile collapses to hamburger, stacked full-width buttons, horizontally scrolling tab row.
- **Take:** **lot number as the identity of a card**; hairline between photo and data; toolbar order (search → filters → sort); category tabs as a horizontal scroller on mobile; filters behind one clear button.
- **Reject:** crowd-photo marketing hero; red glow gradients; low-contrast grey metadata; pink "now inviting consignments" pill.

### Image 4 — Registration packages (light cards on red)
- **Observed:** white cards, large price, tiny grey fine print, icon-led feature rows with a consistent grid, black primary button, mobile cards with a **coloured top rule** (red = premium, grey = classic), disabled features shown with ✕ and grey.
- **Take:** consistent icon+label rows (our spec rows); **top rule colour encodes category/state**; strikethrough/greyed "not included" pattern for unavailable features.
- **Reject:** saturated red page background; "NEW!" badges.

### Image 5 — Vehicle detail (dark, modern)
- **Observed:** chip row of auction facts (Price, No Reserve, Lot, Status, Location) above the title; sticky in-page anchors (Details / Photos / Description / Financing); "Back to results" with previous/next arrows; share icon; single white CTA; full-bleed hero photo on a dark floor; 3-column icon spec grid; masonry photo mosaic of detail shots.
- **Take:** facts chips directly above title; **prev/next through search results** on the detail page; in-page anchors; spec grid; mosaic gallery below the fold.
- **Reject:** price shown as "Request Bidder Info" (our price and bid are the first thing); watermarks.

### How vintage and modern coexist
Shared: type, grid, spacing, radius, controls, money UI, status colours, card anatomy. Different: **photo stage treatment only** (§3.9). Both worlds already share the same skeleton in the references: condensed display type, one red, hairline rules, a lot number, a photo that dominates.

---

## 3. Design system

### 3.1 Tokens — colour
Two global themes (light "paper", dark "showroom"), user-switchable, default `prefers-color-scheme`. Define as CSS variables on `:root` and `:root[data-theme="dark"]`; no hard-coded hex in components.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg` primary background | `#F7F6EC` | `#0E0E10` | page |
| `--bg-2` secondary background | `#EFEDE0` | `#151518` | bands, toolbar, sticky bars |
| `--surface` | `#FFFFFF` | `#1A1A1E` | cards, panels, inputs |
| `--surface-raised` | `#FFFFFF` + shadow | `#222227` | popovers, dialogs, toasts |
| `--ink` primary text | `#1A1916` | `#F2F0EA` | text, key numbers |
| `--ink-2` secondary text | `#4A4741` | `#B4B0A6` | body secondary, labels |
| `--ink-3` muted text | `#6B675E` | `#8E8A82` | metadata only, never for essential info (≥4.5:1 required, verify) |
| `--rule` border | `#D9D6C6` | `#2E2E34` | hairlines, dividers |
| `--rule-strong` | `#1A1916` | `#F2F0EA` | 2px top rules on cards/panels |
| `--accent` brand red | `#A81F1A` | `#E5483E` | primary button fill (`--accent-fill` below), active filter marker, wordmark, links |
| `--accent-fill` button bg | `#A81F1A` | `#C62A22` | white text on both ≥5.5:1 |
| `--live` | `#1F7A47` | `#4DB57E` | live indicator, "You're winning" |
| `--ending` | `#A85F00` | `#E8A23B` | ending soon, outbid, time-extended |
| `--danger` | `#B3261E` | `#FF6B60` | errors, destructive confirm only |
| `--info` | `#2B5C9E` | `#7FA9E6` | neutral notices (reconnecting, queued) |

Rules for the red, because red also reads as "error":
- Brand red appears on: primary CTA fill, wordmark, active-filter tick, link underline hover, focus-adjacent marks. It never signals state.
- **State has its own colours + icon + text**: live=green dot "Live", ending soon=amber clock "Ending soon", outbid=amber, error=`--danger` with an icon and message text. State is never colour alone.
- `--danger` is only ever shown as an inline message with an icon, or the confirm button in a destructive dialog.

Photo-stage palettes (used only on the detail hero stage, §3.9): `--stage-classic: #E9E4D2` (light theme) / `#1B1812` (dark theme); `--stage-modern: #0B0B0D` in both themes.

### 3.2 Typography
Self-host via Fontsource (no third-party CDN), subset Latin, `font-display: swap`, preload the two most used files.

- **Display / headings / lot plates: Barlow Condensed** 600, 700. Chosen from the subject: it descends from US highway and licence-plate signage, which is literally the vernacular of cars. Condensed lets long titles ("1970 Porsche 911 S Targa") fit a card without shrinking.
- **Body / UI / metadata: IBM Plex Sans** 400, 500, 600. Engineered, neutral, strong numerals, genuine tabular figures.
- **Numerals for bids/countdowns:** Barlow Condensed 600 with `font-variant-numeric: tabular-nums lining-nums`. **Verify tabular figures ship in the Fontsource build**; if not, use Plex Sans 600 for numbers. Containers get `min-width` in `ch` so digits never shift layout.
- No monospace for labels. No letter-spaced all-caps labels. Labels are sentence case, 13–14px Plex Sans 500, `--ink-2`.

Scale (rem, 16px root; line-height in parentheses):

| Role | Size | Notes |
|---|---|---|
| `display` (page H1 on detail) | 40/48 → 56 at ≥1024 (1.05) | Barlow 700, sentence/title case as written, never forced upper-case |
| `h2` | 28 (1.1) | Barlow 700 |
| `h3` | 22 (1.15) | Barlow 600 |
| `price-xl` (current bid, detail) | 44 mobile / 56 desktop (1) | Barlow 600 tabular |
| `price-md` (cards) | 24 (1) | Barlow 600 tabular |
| `time-lg` (detail countdown) | 28 (1) | Barlow 600 tabular |
| `body` | 16 (1.55) | Plex Sans 400; max line length 68ch |
| `body-sm` | 14 (1.45) | |
| `meta` (min size anywhere) | 13 (1.4) | Plex Sans 500 |

Titles are written as the seller wrote them: "1970 Porsche 911 S". Do not auto-uppercase.

### 3.3 Spacing
4px base unit. Scale: 4, 8, 12, 16, 24, 32, 48, 64, 96. Page gutters: 16 mobile / 24 tablet / 32 desktop. Max content width 1360px. 12-col grid ≥1024, 8-col tablet, 4-col mobile. Card inner padding 12 (dense) / 16 (default). Section gap 48 desktop / 32 mobile. Whitespace is spent between groups, never between a label and its value.

### 3.4 Radius
Controls (buttons, inputs, chips) **4px**. Cards, photos, panels **2px** (spec-plate, nearly square). Dialogs/sheets **8px**. Full radius only for status dots and avatars. No pill buttons.

### 3.5 Shadow
None on cards. One shadow token for things that float: `0 8px 24px rgba(0,0,0,.14)` light / `0 8px 24px rgba(0,0,0,.5)` dark, used by popovers, dialogs, toasts and the mobile sticky bid bar.

### 3.6 Borders and dividers
1px `--rule` hairlines separate **information groups** (photo/data in a card, rows in a spec table, rows in bid history). 2px `--rule-strong` top rule marks "this is the object you act on" (auction card top edge on hover/focus, bid slip, wallet summary). Top-rule *colour* on a card encodes state (§3.7). No borders just for decoration.

### 3.7 Card state rule
Auction card top rule (2px): `--rule-strong` default; `--live` when you lead; `--ending` when you're outbid or <1h left; `--rule` (hairline) for ended. Always paired with text.

### 3.8 The Bid Slip (signature component)
Heavy 2px top rule, `--surface` card, tight vertical rhythm, four labelled figures in a fixed order, one action. It looks like a clipped lot ticket: right-aligned lot plate, left-aligned figures, no ornament. The price number is the largest thing on the page after the title. Anatomy in §4.4.

### 3.9 Mood: classic vs modern
`data-mood="classic|modern"` is set on the **detail hero stage** and, in small ways, on **cards**. Three levers only:

| Lever | Classic | Modern |
|---|---|---|
| Stage background | `--stage-classic` (warm paper / warm charcoal), photo set in a 12px matte border, like a print | `--stage-modern` (neutral black), photo full-bleed, no border |
| Lot plate | Outlined plate, 1px border, slight inset | Solid tab, flat |
| Title weight | Barlow 700 | Barlow 600, −1% tracking |

Everything else (grid, type, controls, money UI, spacing, status colours) is shared. Grids on Home/Browse never mix backgrounds: cards use the global theme. Only the matte border vs full-bleed crop and plate style vary at card level.

**How mood is derived** (pure function `getMood(item)` in `lib/listing.ts`): `item.type` lower-cased equals `classic` or `modern` → use it; else parse a leading 4-digit year from `item.name`: ≤1989 → classic, ≥1990 → modern; else classic-neutral (paper). Sell flow writes `item.type` as `classic` or `modern` (§4.6). Replace with a real field when **B-2** lands.

### 3.10 Icons and imagery
Lucide, 1.5px stroke, 16/20/24 sizes. Always paired with text for status/action, icon-only only for universally understood controls with `aria-label`. Photo aspect: cards 3:2, hero 16:9 (cap height 70vh), thumbnails 3:2. Missing photo: `PhotoPlaceholder` (neutral plate with year + title, labelled "No photos yet").

### 3.11 Focus, hover, active
- Focus: 2px `--ink` outline + 2px `--bg` offset gap, never removed; on dark `--ink` is light, so it works in both themes.
- Hover: 1-step surface shade or underline. No scale/lift transforms on cards.
- Primary button: `--accent-fill` background, white text, 4px radius, 44px min height (48px for Place bid). Secondary: 1px `--ink` outline, transparent. Tertiary: text with underline on hover.

---

## 4. Information architecture and pages

### 4.0 Routes

| Route | Page | Auth |
|---|---|---|
| `/` | Home / Discover | public |
| `/auctions` | Browse and search (`?q&status&sort&era`) | public |
| `/auctions/:id` | Auction detail and bidding | public to view, login to bid |
| `/sell/new` | Guided listing flow (steps as sub-routes `/sell/new/vehicle`, `/auction`, `/review`) | required |
| `/account` | Dashboard (`/account/bids`, `/selling`, `/saved`, `/wallet`, `/profile`) | required |
| `/saved` | Redirect to `/account/saved` | — |
| `/login`, `/register` | Auth | public only |
| `/activate?token=` | Calls `GET /v1/users/activate` | public |
| `/check-email` | After register / unactivated login | public |
| `*` | 404 | — |

Primary nav (desktop): Auctions · Sell · (search field) · Account / Log in. Mobile: bottom tab bar (§6.4) — Browse, Search, Saved, Account — plus top bar with wordmark. Footer: minimal (about, terms, contact placeholders).

### 4.1 Home / Discover
- **Purpose:** get to a live auction in one look and one click.
- **User goal:** see what is ending, what is hot, what is new.
- **Layout (desktop):**
  1. Global nav with an always-visible search field (typeahead).
  2. **Live strip** (replaces a marketing hero, ≤220px tall): left, one featured auction as a wide card (photo 16:9 + title + current bid + time left + "View lot"); right, a compact "Ending soon" list of 5 rows (thumb, title, current bid, time left). The featured auction = most-bid auction from `GET /v1/auctions/trending?limit=1`, falling back to the soonest-ending active auction.
  3. **Quick filters row:** All · Classic · Modern (era, §3.9) as tabs, plus a "No reserve" absence (all are; omit).
  4. **Ending soon** (row of 4 cards).
  5. **Trending** (row of 4; `trending`).
  6. **Newly listed** (row of 4; `GET /v1/auctions?status=ACTIVE&limit=8`).
  7. **Recently sold** (row of 4; `status=COMPLETED&limit=8`, shows final price).
  8. Footer.
- **Info hierarchy per row:** section title (h2) + "See all" link to `/auctions` with the matching query; cards.
- **Components:** `Navbar`, `SearchBar`, `FeaturedAuction`, `AuctionRow`, `AuctionCard`, `CompactAuctionRow`, `EraTabs`, `Footer`.
- **Interactions:** era tabs filter client-side across the loaded rows (interim, B-3); cards link to detail; hover prefetches `GET /v1/auctions/{id}`.
- **States:** skeleton rows (same card dimensions); empty row hides itself; error shows a one-line inline retry inside the row only (page stays usable).
- **Desktop:** 4 columns of cards. **Tablet:** 3 columns, live strip stacks. **Mobile:** featured full-width card, then rows become **horizontal scroll-snap carousels** with 1.15 cards visible (affordance), section "See all" always visible.
- **Interim data:** "Ending soon" = `status=ACTIVE&limit=100`, sort client-side by `ends_at`, take 8 (labelled in code as interim, **B-4**). Cache 30s; refetch on window focus.

### 4.2 Browse and search (`/auctions`) — most important list page
- **Purpose:** find and compare vehicles fast.
- **User goal:** narrow to something interesting, scan price/bids/time, open it.
- **Layout (desktop ≥1024):**
  - Page title line: "Auctions" + result summary ("Showing 24 loaded"; no totals, API has none).
  - **Toolbar** (sticky under nav, `--bg-2`): search input (typeahead) · era tabs (All, Classic, Modern) · Status select (Live, Starting soon, Ended) · Sort select · "Filters" button (opens panel when more facets exist). Active filters shown as removable chips below the toolbar with a single "Clear all".
  - **Left filter rail (280px)** only when ≥1 facet beyond status exists; today it is hidden and everything fits in the toolbar. The rail structure is built now (§10.5), driven by capabilities.
  - **Grid** 3 columns beside the rail / 4 without, `AuctionCard` list, "Load more" button, loaded count.
- **Filters, tiered:**
  - **Tier 1 (works now):** text search (`/v1/auctions/search`), status (`status`), era (client-side from `getMood`), sort.
  - **Sort options Tier 1:** Newest (API default), Most bids (`/trending` when no query and status=ACTIVE; otherwise hidden), Ending soonest (interim client sort of ≤100 active), Price low/high (client sort of loaded set, marked "Sorted within loaded results" tooltip). When a sort cannot be honest over the full set, the control shows the caveat inline.
  - **Tier 2 (needs B-3):** make, model, year range, bid range, body type, location, transmission, fuel, reserve. Spec the UI now: each facet is a collapsible group in the rail (checkbox list ≤8 + "Show more", range inputs for year/price). On mobile all of these live in a full-height sheet.
- **Search behaviour:** typeahead after 2 chars, 150ms debounce, `GET /v1/auctions/suggest` (≤5). Results page uses `/v1/auctions/search?q&status`. Highlights: server returns `highlight` containing `<em>`; **never inject server HTML**. Split on `<em>` / `</em>` and render `<mark>` React nodes. If `backend: "postgres"` the typo tolerance note is not shown (no UI difference, but record for debugging).
- **URL is the state:** `?q=&status=&sort=&era=` so results are shareable, back-button safe. Filter changes `replace` history while typing, `push` on commit.
- **Pagination:** keyset cursors, `limit=24` (search max 50). "Load more" button; no infinite auto-load by default (keeps footer reachable); retain pages in cache so Back restores position and scroll.
- **States:** loading = 12 skeleton cards; empty = "No auctions match 'x'." + suggestions (clear filters, view all live auctions); error = inline panel "Couldn't load auctions" + Retry; offline banner.
- **Mobile:** toolbar collapses to search field + "Filters" and "Sort" buttons (opening bottom sheets); active filter chips scroll horizontally; era tabs scroll horizontally; grid is 1 column of horizontal cards (photo left 40%, data right) at <480px so more lots are visible per screen, 2 columns of vertical cards at 480–767. **Tablet:** 2–3 columns, filters in a slide-over panel.

### 4.3 Auction card (`AuctionCard`) — the information contract
Show only what the API provides now; reserved slots show when data exists.

Vertical card (default):
```
┌───────────────────────────────┐ 2px top rule (state colour)
│ [photo 3:2]            [Lot ▢]│  lot plate = last 4 of auction id (display only), see below
│ ◔ Ending soon (badge)         │  state badge overlays photo bottom-left
├───────────────────────────────┤ 1px rule
│ 1970 Porsche 911 S            │  title, Barlow 600 20px, max 2 lines
│ Classic · No reserve          │  meta 13px  (era · reserve; location when available)
│                               │
│ Current bid        Time left  │  labels 13px --ink-2
│ $48,500            2d 04h     │  price-md / time 22px, tabular
│ 17 bids                       │  meta
│ ✓ You're the high bidder      │  only if mine; or "Outbid"
└───────────────────────────────┘                       [♡] watch (top-right of photo)
```
- **Hierarchy:** (1) photo, (2) title, (3) current bid and time left (equal weight, side by side), (4) bid count, (5) state/you-status, (6) meta.
- **Lot plate:** the API has no lot numbers. Use the first 4 hex chars of the auction id uppercased, formatted `Lot 7F3C`; replace with a real lot number if **B-9** adds one. The plate is an identity marker, not a sequence.
- **No bids yet:** the price slot reads "Starting bid" with `starting_price`; bid count "No bids".
- **NOT_ACTIVE:** price slot "Starting bid", time slot "Starts in 1d 3h".
- **COMPLETED:** price slot "Sold for $X" (or "No bids" + muted), time slot "Ended Mar 3".
- **CANCELLED:** whole card muted, badge "Cancelled".
- **Whole card is one link** (title is the anchor; `::after` covers the card). Watch button sits above it (separate focus stop). Photo uses `alt` = title; decorative duplicates `alt=""`.
- **Compact horizontal variant** for mobile lists and "Ending soon" lists: 96×64 thumb, title, price, time, 1 line each.
- **Live update:** card values do not use WebSockets in grids (§5.3); they refresh on a 30s refetch and window focus. Time left updates every second from the shared clock.

### 4.4 Auction detail (`/auctions/:id`) — the most important page
- **Purpose:** inspect the vehicle and bid with confidence.
- **User goal:** understand the car quickly; know the state of the auction at a glance; place a bid without doubt.

**Desktop layout (≥1024), 12-col:**
```
┌ Breadcrumb: Auctions / [result prev ‹] [next ›]            [Share] [♡ Watch] ┐
│ chips: ◉ Live · No reserve · Classic · Lot 7F3C · Listed by private seller  │
│ 1970 Porsche 911 S Targa                                          (h1)      │
├──────────────────────────────────────────────┬──────────────────────────────┤
│ STAGE (mood): hero photo 16:9                │ BID SLIP (sticky, 4 cols)    │
│ thumbnail strip beneath                      │  Current bid     $48,500     │
│                                              │  17 bids · 6 bidders          │
│ In-page tabs (sticky): Details · Photos ·    │  Time left       2d 04:12:09 │
│ Description · Bid history                    │  Ends Sat 14 Jun, 7:42 PM    │
├──────────────────────────────────────────────┤  Your bid        —           │
│ Details (3-col spec grid)                    │  Min next bid    $48,750     │
│ Description                                   │  [Bid amount  $______ ]      │
│ Photo mosaic                                  │  [+1] [+2] [+5] increments   │
│ Bid history                                   │  Funds: $62,000 available    │
│ Seller · Related auctions                     │  [ Place bid $48,750 ]       │
└──────────────────────────────────────────────┴──────────────────────────────┘
```
- Stage: classic = matted print on warm paper; modern = full-bleed on black (§3.9). The BID SLIP is **outside** the stage, in the global theme.
- The slip is `position: sticky; top: 72px` and always visible beside the content on desktop.

**Bid slip — the four figures (fixed order, fixed labels, never reordered by state):**

| Label | Value | Source |
|---|---|---|
| **Current bid** | `price-xl`; "Starting bid" + `starting_price` when no bids | `current_bid` or `starting_price` |
| **Time left** | `time-lg` countdown + absolute end time in user's timezone | `ends_at` (server-offset corrected) |
| **Your bid** | your highest bid on this auction or "—"; leader/outbid status line under it | bid history where `user_id == me` / `current_bidder_id` |
| **Minimum next bid** | `min_next_bid` (WS) or `MinimumBid` derived: `current_bid + min_increment`, or `max(starting_price,1)` | snapshot / auction |

Then: the **Bid amount** input (the only editable thing), increment chips, funds line, and one primary button whose label always carries the exact amount: "Place bid $48,750".

> The user brief mentions "YOUR MAX BID". The engine has **no proxy/max bidding** — a bid is the amount charged. The slip therefore shows "Your bid" (your highest bid so far) and a "Bid amount" input. Do not add a max-bid field unless the backend adds proxy bidding (**B-10**); if it does, add a fifth figure "Your max bid" and reword.

**Bid slip details:**
- Input: money input in major units, `inputmode="decimal"`, prefilled with the minimum. Label "Bid amount". Increment chips: "+ minimum", "+2×", "+5×" of `min_increment` relative to `min_next_bid`.
- Under the input, a plain-language consequence line, computed from the real engine rules:
  - New leader: "$48,750 will be held from your balance. It's released if someone outbids you."
  - You lead and raise: "Only the extra $1,250 is held."
- Funds line: "Available balance $62,000" with "Add funds" link when the amount exceeds it (real 422 `insufficient funds` is also handled).
- **Two-step inline confirm** (config `CONFIRM_BIDS`, default on): pressing "Place bid $X" morphs the slip in place into "Confirm bid of $X? [Confirm bid] [Change]". If the minimum rises during confirmation (someone bid), the slip drops back to input with "Someone bid. Minimum is now $Y."
- Logged out: button reads "Log in to bid"; opens `/login?next=` (preserving the entered amount in `sessionStorage`).
- Owner viewing own listing: slip replaced by "This is your listing" with status, cancel (if `NOT_ACTIVE` and before start) and watchers placeholder; bidding disabled (backend would 403).
- Wallet funds unknown/not loaded: skeleton on the funds line, bid not blocked client-side (server is authoritative).

**Below the stage:**
1. **Details** — 3-col spec grid of icon + label + value (interim from parsed listing metadata, §10.3). Rows only appear for known values. If none, section hidden.
2. **Description** — plain text / limited Markdown (headings, lists, bold, links; no raw HTML), 68ch measure, "Read more" past 12 lines.
3. **Photos** — mosaic grid (after B-1); lightbox with keyboard (←/→/Esc), caption, count, swipe on mobile. Hidden entirely until photos exist; stage shows `PhotoPlaceholder`.
4. **Bid history** — newest first: bidder label (Bidder 3 / You), amount, relative time with absolute on hover/long-press; "Load more" via cursor; new live bids slide in at top (§8). Highlights the top row as "High bid".
5. **Seller** — "Private seller" card. Shows "You" for own listing. Reserve slot for future profile (**B-7**).
6. **Related auctions** — 4 cards: other `ACTIVE` auctions with same mood, excluding this one (client-side from the active list; interim).
7. **Facts / how bidding works** — a small collapsed explainer: "Bids are binding. Funds are held while you lead. A bid in the last 2 minutes extends the auction (up to 10 times)." (Values from config, **B-11**.)

**Result navigation (‹ ›):** when arriving from Browse, pass the ordered id list (router state/session) so previous/next works; absent when arriving directly.

**Mobile detail (<768):**
```
┌ ‹ back        Share ♡ ┐
│ photo carousel (swipe) │  full-bleed, 3:2, dot/counter "3/18"
│ chips (scroll-x)       │
│ Title (display 32)     │
│ ◉ Live · Ends in 2d 4h │  one line status
│ Current bid  $48,500   │  price-xl block (not in slip yet; the slip is sticky bottom)
│ 17 bids                │
│ [tabs sticky: Details · Description · Bids] │
│ …sections…             │
└ STICKY BID BAR ────────┘  fixed bottom, 72px + safe-area inset:
   Current $48,500 · 2d 04h        [ Bid $48,750 ]
```
- The sticky bar shows current bid + time left and **one button**. Tap opens a **bottom sheet** containing the full Bid Slip (all four figures, input, chips, funds, confirm). The sheet traps focus, closes with Esc/swipe-down, and keeps the keyboard from covering the button (`visualViewport` aware).
- Detail below the fold keeps a non-sticky mini-summary of the four figures for reading.
- **Tablet (768–1023):** single column; slip is a card between title and stage on portrait; sticky bar as on mobile; two-column details.

**Components:** `AuctionHeader`, `FactChips`, `PhotoStage`, `Thumbstrip`, `Lightbox`, `BidSlip`, `BidAmountInput`, `Countdown`, `PriceDisplay`, `StatusBadge`, `YourStatusLine`, `ConfirmBid`, `BidHistory`, `SpecGrid`, `Description`, `SellerCard`, `WatchButton`, `ShareButton`, `RelatedAuctions`, `StickyBidBar`, `BidSheet`, `ConnectionDot`.

### 4.5 Live / active bidding behaviour
Real-time mechanics and every state are in §5 and §7. Summary of the architecture: one shared WebSocket, version-gated snapshots, REST as fallback and for the write path, server-confirmed money.

### 4.6 Sell (create auction)
Supported fields (API): `item.name`, `item.type`, `item.description`, `starting_price`, `min_increment`, `starts_at`, `ends_at`. Photos and structured specs are not supported yet.

- **Purpose:** list a vehicle with few decisions and no surprises.
- **Layout:** left stepper (desktop) / top progress "Step 2 of 4" (mobile); single column form (max 640px) with a live **preview card** on the right (desktop) showing the exact `AuctionCard` as it will appear.
- **Steps:**
  1. **Vehicle** — Era (Classic / Modern; writes `item.type`), Year, Make, Model, Variant (composed into `item.name`, e.g. "1970 Porsche 911 S"; max 200 chars), spec fields (mileage, transmission, engine, colour, VIN, location) serialised by the interim convention (§10.3), Description (max 5000, counter).
  2. **Photos** — **hidden until B-1.** When it ships: drag/drop, reorder, set cover, min 6 recommended, per-file progress, alt text field.
  3. **Auction** — Starting price, Minimum increment (default presets: 1%/2%/5% of start rounded to a nice number, or custom; "0 means 1" in API, show min 1), Start (Now / schedule), Duration chips (3, 5, 7, 10 days) or custom end; summary "Ends Sat 21 Jun 7:42 PM (your time)". Notes: "A bid in the last 2 minutes extends the auction."
  4. **Review** — read-only summary exactly as buyers will see it (preview card + detail summary), terms checkbox, button "Publish auction".
- **Validation:** inline on blur and on step "Continue"; API 400 `fields` map keys (`item.name`, `ends_at`…) mapped to the matching inputs and the first error focused; error summary at top of the step on submit.
- **Draft:** form state autosaved to `localStorage` (per user) so a refresh doesn't lose work; "Discard draft" action. No server drafts exist.
- **Success:** `201` → success page with the auction link, "View listing", "List another". If `starts_at` is in the future the status is `NOT_ACTIVE`; copy explains "You can cancel until it starts."
- **Mobile:** the same steps, full-screen, sticky "Continue" at the bottom; preview card is a collapsible section on Review only.
- **States:** logged out → redirect to login with `next`; not activated → blocked with "Activate your account to list" (login would already be blocked); `401` on submit → refresh token then retry once.

### 4.7 Dashboard (`/account`)
Plain tabbed list, **no analytics, no charts**. Tabs: **Bids**, **Selling**, **Saved**, **Wallet**, **Profile**.
- **Bids:** segmented control Leading · Outbid · Won · Lost. Each row = compact auction row + your bid + status + time left. Data via the interim ledger derivation (§4.8). Active ones are live-subscribed (≤20).
- **Selling:** `GET /v1/auctions?owner=me`, grouped Scheduled / Live / Ended / Cancelled; row actions: View, Cancel (only `NOT_ACTIVE` before `starts_at`, confirm dialog).
- **Saved:** device-local watchlist (§4.9).
- **Wallet:** summary slip (Available · Reserved · Total, from `GET /v1/wallet`), "Add funds" (labelled "Test funds" in non-production), and the **statement**: ledger entries paged via `before`, each row: date, kind in words (Deposit, Held for a bid, Released, Settled), auction title link, amount, account.
- **Profile:** email, activated status, log out, theme toggle.
- **Mobile:** tabs become a horizontally scrolling segmented control; rows are the compact variant. **States:** each tab has loading/empty/error ("You haven't placed any bids. Browse live auctions.").

### 4.8 Deriving "My bids" from the wallet ledger (interim, B-6)
The ledger is per user and includes `auction_id` on RESERVE / RELEASE / SETTLE entries. Because every accepted bid reserves funds and being outbid releases them:
- Auction with RESERVE and net reserved >0 and `status=ACTIVE` → **Leading**.
- RESERVE followed by RELEASE with `status=ACTIVE` and no net reserve → **Outbid**.
- `COMPLETED` and `current_bidder_id == me` → **Won**; `COMPLETED` with a release and not leader → **Lost**.
Implementation: page the ledger (`limit=200`), reduce to a `Map<auctionId, {netReserved, lastAt}>`, hydrate unique auctions with `GET /v1/auctions/{id}` (cached, batched ≤10 concurrent). Treat the live `auction.current_bidder_id` as the authority for leader status; the ledger only tells you *which* auctions you participated in. Replace with `GET /v1/users/me/bids` when it exists.

### 4.9 Watchlist
- **Interim:** `localStorage` set of auction ids per signed-in user (and anonymous bucket), labelled "Saved on this device". Heart button (`aria-pressed`) on cards and detail; `/account/saved` lists them as cards with a status filter.
- **Honesty:** copy says "Saved on this device" in the empty state and under the tab title. When **B-5** lands, migrate local ids to the server on next login and remove the label.
- **No watcher counts** (no data).

### 4.10 Authentication
- **Register:** email, password (min **15**, max 128 — say "At least 15 characters"; no composition rules; show length counter, allow paste, show/hide toggle, `autocomplete="new-password"`). `409` → "An account with this email already exists. Log in or reset…" (reset only once B-8 exists; for now "Log in"). `429` → show `Retry-After`. After success → `/check-email` ("We sent an activation link to x"), with "Send again" (always `204`; tell the user "If this address has an account, we've sent a new link" — mirrors the backend's no-enumeration design).
- **Activate:** `/activate?token` auto-calls the endpoint; success → "Account active. Log in."; failure → "This link has expired or was already used. Send a new one."
- **Login:** email, password, `autocomplete` attributes, "Keep me signed in" checkbox. `401` → "Email or password is incorrect." (never specify which). `403` → "Activate your account first" + resend. `429` → countdown from `Retry-After`.
- **Session:** access token in memory only; refresh token in `sessionStorage` (or `localStorage` if "Keep me signed in"). Single-flight refresh: on any `401`, call `/v1/auth/refresh` once, retry the original request once; failure → clear session, redirect to login with `next`. Proactive refresh at ~80% of access TTL (15 min default → refresh at ~12 min). Cross-tab sync via `BroadcastChannel`/`storage` event (logout in one tab logs out all). Logout calls `/v1/auth/logout` (204) then clears local state.
- **Protected routes:** wrapper checks session; during bootstrap show a neutral skeleton, never a flash of the login page. Logged-out bid attempts preserve intent (§4.4).
- **Forgot password:** **not shipped** (no API; **B-8**). Do not render a dead link.
- **Errors:** field-level with `aria-describedby`; form-level summary with `role="alert"` for server errors.

### 4.11 Search results page
Same as Browse (§4.2) with `q` set. Result card shows highlighted snippet (`<mark>`). Empty: "No results for 'x'. Check the spelling or try fewer words." plus "View live auctions". Error and loading as Browse. `-exclude` and `"phrase"` are supported by the API: hint text under the field on focus ("Use quotes for phrases, - to exclude").

---

## 5. Real-time and bidding behaviour

### 5.1 Data and clock
- **Server clock offset:** compute from the HTTP `Date` header of the auction fetch (`offset = serverDate − clientNow` at response time, midpoint-corrected). All countdowns use `Date.now() + offset`. One shared 1 Hz ticker (`useNow`) powers every countdown on the page; components memoised by displayed string so React re-renders only when text changes.
- **Formats:** >48h "3d 4h"; 24–48h "1d 6h 12m"; <24h `HH:MM:SS`; <1h `MM:SS` with `--ending`; <2min `MM:SS` bold + "Final minutes — a bid extends the auction". Zero → "Ended" (stays "Ending…" until a snapshot/REST confirms `COMPLETED`, max 5 s, then refetch).

### 5.2 WebSocket manager (`lib/realtime.ts`, one per tab)
- Endpoint `/v1/ws` (same origin via proxy/Caddy). Messages: client `{action:"subscribe"|"unsubscribe", auction_id}`; server `subscribed`, `unsubscribed`, `auction.updated {cause, auction:{id,status,current_bid,current_bidder_id,bid_count,min_next_bid,ends_at,extensions,version}}`, `error`.
- **Ref-counted subscriptions** per auction id; resubscribe all on reconnect.
- **Version gate:** keep `lastVersion[id]`; drop snapshots with `version <= lastVersion`. REST `Auction` has **no `version`** today (**B-12**): REST data may overwrite cache only if no WS version has been seen for that id, or if `updated_at` is newer than the last WS-applied time.
- **Merge:** apply snapshot fields into the TanStack Query cache entry for `["auction", id]` and the history list. A snapshot changes `current_bid`, `current_bidder_id`, `bid_count`, `ends_at`, `extensions`, `status`, and `min_next_bid` (store `min_next_bid` in the cached auction).
- **History on bid:** when `cause` is `bid.placed` and `bid_count` rose, `invalidate` page 1 of history (debounced 250 ms) rather than trusting deltas; the snapshot carries no bid row.
- **Reconnect:** exponential backoff 500 ms → 10 s with ±30% jitter. Close codes: 1001 reconnect fast (200–800 ms jitter); 1013 back off ≥5 s; 1008 (too slow) reconnect and refetch; 1009 log. On every (re)connect: resubscribe and **refetch auction + history page 1** (pub/sub is lossy by design).
- **Fallback polling:** if disconnected >10 s while an auction is on screen, poll `GET /v1/auctions/{id}` every 5 s until the socket returns; stop on reconnect.
- **Visibility:** when tab hidden, keep the socket; on `visibilitychange→visible` refetch. No animations while hidden.
- **Scope:** subscribe only on the detail page and Dashboard → Bids (≤20). **Grids do not subscribe**; they refetch every 30 s and on focus. Open question: server-side max subscriptions per connection (§12 Q4).
- **Connection indicator** (`ConnectionDot` in the bid slip header): green dot "Live"; amber "Reconnecting…"; grey "Offline". It is text + dot, `aria-live=polite`.

### 5.3 Place-bid flow
1. Client validates: amount is a number, ≥ `min_next_bid` (in minor units), ≤ safe integer. Not > wallet if known (soft warning only; server decides).
2. Generate `Idempotency-Key = crypto.randomUUID()` for this **intent**. Reuse the same key for retries of the same amount; new key if amount changes.
3. `POST /v1/auctions/{id}/bids {amount}` (minor units). Default **synchronous** (no `Prefer`); if the gateway returns `202` with a `BidRequest`, poll `GET /v1/bid-requests/{id}` every 500 ms up to 15 s, showing "Bid queued…".
4. Pending: button shows spinner + "Placing bid…", input disabled; **no optimistic leader/price change.**
5. `201`/`200` → success state (§7); apply `current_bid`, `bid_count`, `ends_at`, `extended` immediately from the response (it is server-confirmed) and let the snapshot confirm; invalidate history and wallet.
6. Network error / timeout (outcome unknown): show "Confirming your bid…", retry once with the **same key** after 1 s (safe: idempotent), then refetch history to see whether it landed; if still unknown, show "We couldn't confirm your bid. Check bid history before trying again."

### 5.4 Error mapping for place-bid

| Status | Meaning | UI |
|---|---|---|
| 401 | Not logged in / token expired | Refresh once; else login with preserved amount |
| 403 | Your own auction | Replace slip with "This is your listing" |
| 404 | Auction gone | "This auction is no longer available." + link to browse |
| 409 | Not open / ended | Refetch; slip switches to ended/upcoming state; "This auction isn't open for bids." |
| 422 + `minimum_amount` | Too low (someone bid first) | Update minimum, set input to it, inline: "Minimum bid is now $X." |
| 422 insufficient funds | Not enough balance | Inline: "You need $X more to bid $Y." + "Add funds" |
| 422 other (`invalid amount`, key mismatch) | Client bug | Generic inline error, log, regenerate key |
| 429 | Rate limited | Disable button for `Retry-After` with countdown "Try again in 8 s" |
| 503 | Contention / bidding unavailable | Auto-retry once with same key; then "Bidding is busy. Your bid wasn't placed. Try again." |
| 5xx / network | Unknown | See §5.3 step 6 |

---

## 6. Responsive behaviour

Breakpoints: mobile 0–639, tablet 640–1023, desktop 1024–1439, wide ≥1440 (content max 1360 centred).

### 6.1 Navigation
- **Desktop:** top bar 64px: wordmark, Auctions, Sell, search (grows), Saved heart, Account/Log in. Sticky.
- **Tablet:** same, search collapses to an icon opening a full-width field.
- **Mobile:** 56px top bar (wordmark, search icon, avatar) + **bottom tab bar** 56px + safe-area: Browse, Search, Saved, Account. Hidden while the bid sheet is open. The detail page replaces the tab bar with the sticky bid bar.

### 6.2 Filters
Desktop rail / tablet slide-over / mobile full-height bottom sheet with a sticky footer "Show results" (no live count; API has none) and "Clear".

### 6.3 Auction cards
4 → 3 → 2 → 1(compact horizontal) columns. Touch targets ≥44px, heart button 44×44 hit area even if the glyph is 20px.

### 6.4 Countdown
Same component everywhere; smaller sizes on cards; on mobile detail it lives in the sticky bar and in the sheet. Ending soon pulses once (not continuously) when crossing the 2-minute threshold.

### 6.5 Gallery
Desktop: stage + thumbstrip, click opens lightbox. Mobile: swipe carousel with counter; tap opens full-screen lightbox with pinch-zoom; the mosaic below the fold becomes a 2-column grid.

### 6.6 Bid history
Desktop: table rows (bidder, amount, time). Mobile: two-line rows (amount + bidder / time). "Load more" in both.

### 6.7 Bid panel
Desktop sticky slip; mobile sticky bar + sheet (§4.4). The amount input is never hidden behind the keyboard.

---

## 7. UX states (exhaustive)

For each: **trigger → UI → copy**.

### 7.1 Data and system
| State | UI | Copy |
|---|---|---|
| Loading | Skeletons with identical geometry (no layout shift); slip shows skeleton figures | — |
| Empty (browse) | Illustration-free message + actions | "No auctions match your filters." [Clear filters] |
| Empty (history) | Inline | "No bids yet. Be the first." |
| Empty (my bids) | Inline | "You haven't placed any bids." [Browse live auctions] |
| Error (page) | Panel, retry | "Couldn't load this auction. Check your connection and try again." [Try again] |
| Auth required | Login prompt | "Log in to bid." |
| Network disconnected | Persistent top banner + slip disabled | "You're offline. Bidding is paused." |
| Reconnecting | `ConnectionDot` amber + banner after 5 s | "Reconnecting to live updates…" |
| Reconnected | Dot green; toast only if offline >10 s | "Back online. Updated." |

### 7.2 Auction lifecycle (from `status`)
| State | UI |
|---|---|
| Upcoming (`NOT_ACTIVE`) | Badge "Starts in 1d 3h"; slip shows starting bid + `starts_at`; button disabled "Bidding opens Sat 7:00 PM"; watch enabled |
| Active (`ACTIVE`) | Badge ◉ Live; full slip |
| Ending soon (<1h) | Badge amber "Ending soon"; time in `--ending` |
| Final minutes (<2 min) | Slip adds line "Final minutes: a bid extends the auction" |
| Extended | Inline notice in slip + history marker: "Time extended. Now ends 7:44:10 PM (extension 3 of 10)." Announced politely |
| Just ended | Slip locks for ≤5 s showing "Auction ended. Finalizing…" until status confirmed |
| Ended with sale (`COMPLETED`, `current_bid`) | Slip becomes result block: "Sold for $X" · final bids count · ended time |
| Ended, no bids | "Ended with no bids." |
| Cancelled | "Cancelled by the seller." Bidding closed |
| Reserve | Chip "No reserve" (all auctions today). Component supports `met` / `not_met` / hidden for future (B-13) |

### 7.3 My position in the auction
| State | Trigger | UI |
|---|---|---|
| Not bid | No bids by me | "Your bid —"; button "Place bid $min" |
| Highest bidder | `current_bidder_id == me` | Green line "✓ You're the high bidder at $X"; button reads "Raise bid"; consequence line explains only difference is held |
| Outbid | I have bids, leader ≠ me | Amber banner "You've been outbid. Current bid is $X." with a one-tap "Bid $min"; held funds released message "Your $Y was released." |
| Won (ended) | `COMPLETED`, leader == me | Result block "You won this auction at $X." + settlement status: `settled_at` null → "Settling payment…"; set → "Settled" |
| Lost (ended) | `COMPLETED`, I bid, leader ≠ me | "You didn't win. $Y was released to your balance." |

### 7.4 Bid action
| State | UI / copy |
|---|---|
| Validating (client) | Inline under input: "Enter at least $48,750." |
| Confirm | "Confirm bid of $48,750?" [Confirm bid] [Change] |
| Pending | "Placing bid…" |
| Queued (202) | "Bid queued. Waiting for confirmation…" |
| Accepted | Toast + slip flash: "Bid placed. You're the high bidder." (action name stays "bid", the button said "Place bid") |
| Accepted + extended | Add: "Time extended to 7:44 PM." |
| Rejected: too low | "Minimum bid is now $X." (input updated) |
| Rejected: funds | "You need $X more." [Add funds] |
| Rejected: ended | "This auction has ended." |
| Rate limited | "Too many bids. Try again in 8 s." |
| Replayed (`200`, `replayed:true`) | Treat as success silently |

---

## 8. Motion

Principle: nothing moves unless it tells the user something changed. All durations ≤200 ms except the price flash. Everything is disabled under `prefers-reduced-motion` (state changes become instant plus a static highlight).

| Moment | Motion |
|---|---|
| Current bid changes | 600 ms background highlight behind the number (accent-tinted, fades), the number swaps instantly (no rolling digits) |
| New bid in history | Row inserts at top with 160 ms height/opacity transition |
| Countdown | No animation each second. One-time pulse at 2-minute threshold |
| Time extended | 600 ms highlight on the time figure |
| Slip → confirm | 160 ms crossfade/height; focus moves to "Confirm bid" |
| Photo change | 150 ms crossfade; lightbox 200 ms fade; thumbnails no transition |
| Bid sheet (mobile) | 220 ms slide-up, ease-out |
| Toasts | 160 ms fade/slide, 5 s (8 s for errors), dismissible, pause on hover/focus |
| Hover/focus | colour/underline only, ≤120 ms |
| Page load | **None** (no staggered entrances) |
| Route change | Instant; scroll to top; focus to `h1` |

---

## 9. Accessibility

Target **WCAG 2.2 AA**.

- **Keyboard:** everything reachable and operable; logical order; skip link ("Skip to results"); Esc closes dialogs/sheets/lightbox and returns focus to the trigger; lightbox ←/→; search suggestions use the combobox pattern (arrow keys, Enter, Esc).
- **Focus:** visible 2px ring on every control (§3.11); no focus loss on live updates; route change focuses the `h1`.
- **Contrast:** text ≥4.5:1, large text/UI ≥3:1 in both themes; `--ink-3` only on non-essential metadata; verify every token pair in CI with a script. Never colour alone (icon + text on every state).
- **Live regions:** a single polite region `#bid-announcer` in the detail page. Announce: bid placed, outbid, you're now leading, extension, auction ended. **Do not** announce each second or every foreign bid; throttle to ≥3 s apart. Time warnings announced at 5 min, 1 min, 30 s, 10 s remaining (`role="status"`). Errors from the user's action use `role="alert"`.
- **Countdown semantics:** `<time datetime>` with a visually hidden full-text version ("2 days 4 hours") and `aria-hidden` on the ticking digits; no per-second announcements.
- **Forms:** visible `<label>` for every input, hints via `aria-describedby`, errors linked and focused, `autocomplete` tokens, no placeholder-as-label, `inputmode` for money, error summary on submit.
- **Bidding interaction:** the amount field, consequence line and button are in one `form`; Enter submits only the first step; Confirm button gets focus when the step changes; the slip announces state with `aria-live` rather than moving focus away.
- **Touch targets:** ≥44×44 CSS px, ≥8px apart.
- **Reduced motion:** honour `prefers-reduced-motion`; no auto-playing media.
- **Images:** meaningful `alt` ("1970 Porsche 911 S, front three-quarter"), gallery items keyboard-focusable with counts, decorative placeholders `alt=""`.
- **Zoom/reflow:** works at 200% zoom and 320px width without horizontal scroll.
- **Language/title:** `lang="en"`, unique `<title>` per route ("1970 Porsche 911 S — current bid $48,500").
- **Cookie-free, no autoplay, no captcha** assumed.
- Audit tooling: `eslint-plugin-jsx-a11y`, axe in Playwright on every route in both themes, manual screen-reader pass (VoiceOver + NVDA) on the bid flow.

---

## 10. Component and code architecture

### 10.1 Stack (recommendation; backend CORS already targets `:5173`)
- **Vite + React 19 + TypeScript (strict)**; **React Router**; **TanStack Query** (server state, infinite queries, cache merge for WS); **React Hook Form + Zod**; **Radix UI primitives** (Dialog, Popover, Tabs, Select, Toast, Tooltip, VisuallyHidden) for accessible behaviour; **Tailwind CSS v4 with the token variables from §3**; **Lucide**; **Fontsource** (Barlow Condensed, IBM Plex Sans); `Intl.NumberFormat` / `Intl.DateTimeFormat` / `Intl.RelativeTimeFormat`.
- Types generated from `/v1/openapi.json` with `openapi-typescript` (check in the generated file); hand-write only the WS message types.
- Tests: Vitest + Testing Library (units), Playwright (e2e against `make up`, plus axe).
- Dev: Vite proxy `/v1` and `/v1/ws` (ws: true) → `http://localhost:4000` to avoid CORS entirely. Production: build static files served by Caddy on the same origin as the API (WebSocket origin allow-list satisfied, no CORS).
- **Directory:** `web/` at repo root. No Next.js: the app is an authenticated, real-time SPA; SEO for listings is deferred (§12 Q7).

### 10.2 Folder structure
```
web/src/
  app/            router, providers (Query, Auth, Theme, Clock, Toasts), error boundary
  routes/         one file per route (thin; compose features)
  features/
    auth/         AuthProvider, session.ts, LoginForm, RegisterForm, RequireAuth
    auctions/     api.ts, queries.ts, AuctionCard, AuctionGrid, FilterBar, SearchBar, StatusBadge
    detail/       PhotoStage, Lightbox, SpecGrid, BidHistory, SellerCard, StickyBidBar, BidSheet
    bidding/      BidSlip, BidAmountInput, ConfirmBid, placeBid.ts, bidMachine.ts, errors.ts
    wallet/       WalletSummary, DepositDialog, Statement, queries.ts
    sell/         SellWizard, steps/*, preview, draft.ts
    watchlist/    useWatchlist.ts, WatchButton
    account/      tabs
  lib/
    api.ts        fetch wrapper (auth header, refresh single-flight, error normaliser, Retry-After)
    realtime.ts   WS manager (ref-counted subs, version gate, backoff)
    clock.ts      server offset + useNow()
    money.ts      parse/format minor units
    listing.ts    parseListing(), getMood(), composeItem()  ← the ONLY place that knows interim conventions
    capabilities.ts feature flags from backend capabilities (§10.5)
    bidState.ts   derive my-position state from auction + history
  components/ui/  Button, Input, Select, Chip, Dialog, Sheet, Toast, Tabs, Skeleton, EmptyState, Pagination(LoadMore), Tooltip, Icon
  styles/         tokens.css, base.css, fonts
```

### 10.3 Interim listing convention (`lib/listing.ts` only)
- **Title:** `item.name` is "YYYY Make Model Variant". `parseTitle` extracts a leading 4-digit year and the remainder; no make/model split (impossible reliably), so no make/model facets.
- **Specs:** the sell flow appends a fenced block to `description`:
  ```
  <description text>

  ---specs---
  mileage: 62,400 mi
  transmission: 5-speed manual
  ...
  ---end---
  ```
  `parseListing(description)` returns `{ text, specs[] }`; the block is stripped from the displayed description. Unknown/absent → no spec grid.
- **Mood:** `item.type` of `classic`/`modern`, else year heuristic (§3.9).
- Every consumer imports from `lib/listing.ts`; when **B-2** adds real fields, only this file and the sell step change.

### 10.4 Core component responsibilities

| Component | Responsibility |
|---|---|
| `Navbar` / `BottomTabs` | Navigation, search entry, auth state, theme toggle |
| `AuctionCard` | Presentational: title, photo, price, time, bids, state; takes `Auction`, `variant`, `myStatus?` |
| `AuctionGrid` | Responsive grid + skeleton + load more; no data fetching of its own |
| `FilterBar` / `FilterPanel` / `FilterChips` | Capability-driven filters; owns URL state via `useSearchParams` |
| `SearchBar` | Combobox typeahead (`suggest`), debounced |
| `Countdown` | Pure: `endsAt`, `now` → formatted text, `<time>`, thresholds; no timers inside |
| `PriceDisplay` | Formats minor units; sizes `xl/md/sm`; label slot; `aria-label` with full words |
| `StatusBadge` | Maps status+time to icon+text+colour (single source for §7.2) |
| `BidSlip` | Container; reads `useAuction`, `useMyPosition`, `useWallet`; renders figures + form |
| `BidAmountInput` | Money field with min, increments, validation messages |
| `ConfirmBid` | Inline confirm step; resets on min change |
| `bidMachine` | `idle → editing → confirming → submitting → (queued) → success | error(kind)`; pure reducer, unit tested |
| `BidHistory` | Cursor list, live insert, anonymised labels |
| `PhotoStage` / `Thumbstrip` / `Lightbox` | Gallery with placeholder, keyboard, swipe |
| `SpecGrid` | Icon + label + value rows from parsed specs |
| `SellerCard` | Seller placeholder / own-listing actions |
| `WatchButton` | Toggle, `aria-pressed`, device-local storage hook |
| `ConnectionDot` | WS state indicator |
| `Toast` / `ConfirmationDialog` / `Sheet` / `Modal` | Radix-based, consistent focus handling |
| `EmptyState` / `Skeleton*` / `ErrorPanel` | State presentation, geometry matched to real content |
| `LoadMore` | Cursor pagination control with loading and "no more" states |

### 10.5 Capability-driven UI (`capabilities.ts`)
A static object today (all false), later fetched from the backend (e.g. `GET /v1/config`):
```ts
{ photos:false, structuredSpecs:false, reserve:false, facets:[], serverSort:false,
  watchlist:false, myBids:false, sellerProfile:false, forgotPassword:false,
  proxyBidding:false, snipeWindowSeconds:120, maxExtensions:10, currency:{code:"USD",minorUnit:2} }
```
Components check flags to show/hide: photo step in Sell, filter groups in the rail, sort options, reserve chip states, "Your max bid" figure, forgot-password link. This is how the plan stays honest now and upgrades without redesign. Currency and minor unit come from `VITE_CURRENCY` / `VITE_MINOR_UNIT` until the backend provides them.

### 10.6 Money
All amounts are integers in minor units. `money.ts`: `formatMoney(minor)` (no decimals when the value is a whole major unit; else 2), `parseMoney(input)` → minor (reject NaN, >2 decimals, > `Number.MAX_SAFE_INTEGER`). Bid input uses major units; a bid of `$48,750` is sent as `4875000`.

---

## 11. Performance

Image-heavy and real-time, so:

- **Budgets:** LCP ≤2.5 s (4G, mid-range phone) on Home/Browse/Detail; CLS ≤0.05; INP ≤200 ms; initial JS ≤180 KB gzip per route (route-level code splitting; Sell, Lightbox and Dashboard lazy-loaded).
- **Images (needs B-1 + an image pipeline/CDN):** serve AVIF/WebP with `srcset` at 400/800/1200/1920 and correct `sizes`; always `width`/`height` or `aspect-ratio` to prevent shift; `loading="lazy"` + `decoding="async"` for everything except the first card row and the detail hero (`fetchpriority="high"`); low-quality blurhash/dominant-colour placeholder; preconnect to the image origin; gallery loads the next/previous image on idle; mosaic thumbnails are small variants, full-size only in the lightbox.
- **Fonts:** two families, subset, `font-display: swap`, preload the two critical weights, `size-adjust` fallback metrics to avoid reflow.
- **Lists:** keyset "Load more" with TanStack `useInfiniteQuery`; windowing not needed at ≤200 cards, add `content-visibility: auto` on cards.
- **Real-time efficiency:** one socket; subscribe only for visible detail and ≤20 dashboard auctions; merge snapshots into the Query cache with a selector so only affected components re-render; one shared 1 Hz clock; `Countdown` memoised by output string; history updates are debounced.
- **Caching:** list queries `staleTime` 15–30 s; detail `staleTime` 5 s while a socket is live (cache is kept fresh by snapshots); hover/focus prefetch of detail; HTTP caching delegated to the server.
- **Skeleton loading** everywhere with exact geometry. **No layout shift** from banners (reserve space, or overlay).
- **Network resilience:** retry idempotent GETs 2× with backoff; never auto-retry a POST except bids with the same idempotency key (§5.3).
- **Measurement:** Lighthouse CI on Home, Browse, Detail (mobile preset); Web Vitals logged to the console in dev.

---

## 12. Backend dependencies and open questions

### Backend work the frontend needs (ordered by impact)
| ID | Need | Why |
|---|---|---|
| **B-1** | Photo storage + `item.images[]` (url, width, height, alt, order) and an upload endpoint | Photos are the product's emotion; sell flow step 2; gallery |
| **B-2** | Structured vehicle attributes (`year`, `make`, `model`, `variant`, `mileage`, `transmission`, `fuel`, `body`, `colour`, `vin`, `location`, `era/category`) | Specs grid, facets, mood, removes interim convention |
| **B-3** | List/search filters for those attributes + facet counts | Make/year/body/etc. filters |
| **B-4** | `sort=ends_at|price|bid_count` on list/search | Honest "Ending soon" and price sort |
| **B-5** | Server watchlist (`PUT/DELETE /v1/watchlist/{auctionId}`, list) | Cross-device saved |
| **B-6** | `GET /v1/users/me/bids` (auctions I bid on with my highest bid and state) | Replace ledger derivation |
| **B-7** | Seller display name / public profile | Trust on detail |
| **B-8** | Forgot / reset password endpoints | Auth completeness |
| **B-9** | Lot numbers per auction | Real lot plates |
| **B-10** | (Optional) proxy/max bidding | Only if wanted; changes the slip |
| **B-11** | `GET /v1/config` (snipe window, max extensions, currency + minor unit, capabilities) | Remove hard-coded constants |
| **B-12** | `version` field on REST `Auction` | Safe REST/WS merge |
| **B-13** | Reserve price (met/not met) | Reserve states |
| **B-14** | Bidder display handles (stable pseudonym) rather than raw `user_id` | Nicer history |

### Open questions for the product owner
1. Is "No reserve" a correct statement for every auction (does any bid ≥ starting price always sell)?
2. Currency and minor unit (README says "paise/cents")?
3. Is "Test funds" deposit user-facing in production, or only in dev/staging?
4. Max WebSocket subscriptions per connection (to decide whether grids could subscribe)?
5. Should the 202 async-bid path be used at all from the browser, or is sync-only acceptable?
6. Refresh token storage: sessionStorage/localStorage (current API returns it in JSON) vs an httpOnly cookie later — acceptable?
7. Is SEO/shareable previews for auction pages required (would motivate SSR or prerender and OG tags)?
8. Image hosting/CDN choice (S3 + CDN, Cloudflare Images, etc.)?
9. Is an admin UI in scope? (Endpoints exist; this plan excludes it.)
10. Terms/legal copy for "bids are binding" and fees: who provides it?

---

## 13. Review against the skills

**`/frontend-design` checks**
- *Subject-grounded:* condensed highway/licence-plate lineage type; lot plates; spec-plate cards; matted print vs full-bleed stage; the Bid Slip as a lot ticket.
- *Defaults avoided or consciously justified:* palette is ivory/oxblood/black because the **brief pinned it** (reference 2 supplies the triad); red is kept off state signals; no eyebrow all-caps labels, no mono labels, no "→" on links, no numbered markers (sell steps are a true sequence, so numbering is legitimate there), no single-word accent in headlines, no glass, no gradient washes, no uniform rounded-card SaaS kit (radius is differentiated: 2/4/8), no staggered entrance animation, hero is the live auction not a banner.
- *One bold thing:* the Bid Slip. Everything else is quiet.
- *Copy:* plain verbs, sentence case, errors say what happened and what to do, one action keeps one name ("Place bid" → "Bid placed").
- *Quality floor:* responsive from 320px, visible focus, reduced-motion, contrast verified per token pair.

**Judgement calls to revisit during implementation**
- Barlow Condensed tabular numerals (verify; fallback to Plex Sans).
- Whether two-step confirm slows last-second bidding too much; make it a config flag and test with real users.
- The 2-theme (light/dark) matrix doubles visual QA; screenshot both in CI.

**`/ui-ux-pro-max`:** unavailable; replaced by the checklist above. Re-run when installed.

---

## 14. Testing and quality gates
- Unit: `bidMachine`, `money`, `listing`, `bidState`, `clock`, `realtime` version gate, error mapping.
- Integration (Vitest + MSW): place-bid error table (§5.4), outbid transition, extension notice, reconnect refetch.
- E2E (Playwright, against `make up`): register → activate (via Mailpit API) → login → deposit → bid → get outbid by a second context → win; mobile viewport run of the same; axe on every route in both themes.
- Visual: Playwright screenshots of Card, Slip (every state), Detail (classic and modern), at 375/768/1280, light and dark.
- CI: lint, typecheck, unit, build, Lighthouse CI budgets.

---

## Implementation Priority

1. **Foundations:** `web/` scaffold, tokens/fonts/themes (§3), `lib/api` (auth header, refresh single-flight, error normaliser), money, clock, generated OpenAPI types, capability flags, Vite proxy.
2. **Auth + wallet minimum:** login, register, check-email, activate, session handling; wallet balance and "Add funds".
3. **Auction detail and bidding (the core):** `AuctionCard` skeleton, detail page, `Countdown`, `PriceDisplay`, **Bid Slip** with `bidMachine`, full error mapping, confirm step, mobile sticky bar and sheet.
4. **Real-time:** WebSocket manager, version gate, reconnect/fallback polling, live history, outbid/winning/extended/ended states, announcer region.
5. **Browse and search:** `AuctionCard` final, grid, toolbar, search + typeahead, status/era/sort (Tier 1), "Load more", URL state.
6. **Home:** live strip, Ending soon / Trending / New / Recently sold rows.
7. **Dashboard:** Bids (ledger-derived), Selling with cancel, Wallet statement, Profile.
8. **Sell flow:** guided steps (without photos), draft autosave, API error mapping.
9. **Watchlist (device-local)** and result prev/next.
10. **Polish and audit:** accessibility pass (`design:accessibility-review`), copy pass (`design:ux-copy`), performance budgets, dark-theme QA, visual regression.
11. **Backend-driven upgrades as B-items land:** photos (B-1) → structured specs/facets (B-2, B-3) → server sort (B-4) → server watchlist and my-bids (B-5, B-6) → config endpoint, reserve, seller profile.
