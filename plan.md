# Frontend plan: auctionEngine

A premium automotive auction house on top of the existing Go auction engine. This document is the design and implementation plan only. No application code, components or dependencies were created.

Rule that settles every disagreement below: **car = emotion, UI = clarity, auction = urgency and trust.** When "more impressive" and "easier to browse or bid" pull apart, the second wins.

---

## 0. Inputs, and what this plan is based on

| Input | Status | Notes |
|---|---|---|
| Repository | Read | `github.com/Ayush1388/auctionEngine`, `main` at v1.0 (commit `96c4e78`). The local checkout in `Desktop/auctionEngine` is older (pre-bidding README, no `api/`), so pull before implementing. |
| API contract | Read | `api/openapi.json`, plus handlers, bidding rules, realtime hub, config, migrations. |
| Existing frontend code | Read | None in the repo. Local-only prototypes exist: `aurelian-landing/`, `aurelian-duality/` (Vite 8, React 19, TypeScript, Tailwind 4, GSAP) and `watch/` (react-three-fiber). They are watch-themed showpieces, not an app. Their stack is reused; their tokens `paper #ECEBE7` and `ink #14130F` are carried over. |
| `/frontend-design` skill | Read and applied | Subject-grounded choices, one bold element, restraint, copy rules, the list of generated-design tells to avoid. |
| `/ui-ux-pro-max` skill | Read and applied | Priority table, full quick reference (10 categories), and searches against its database. Results used and results rejected are listed in section 14. |
| Reference images | **Not received** | No images reached this session, only the written brief. Section 2 extracts principles from the brief's written description of the two reference sets. It must be re-checked against the actual images before implementation; anything in section 2 that the images contradict loses. |

### 0.1 What the backend actually supports

The frontend must not invent backend behaviour. This table is the ground truth for the rest of the plan.

| Capability | Exists today | Detail |
|---|---|---|
| Register, activate by email, resend activation, login, me | Yes | Password minimum 15 characters, maximum 128. Login returns 403 until activated. |
| Sessions | Yes | Access JWT (15 min by default) plus rotating refresh token (30 days). Reusing an old refresh token returns 401. Tokens travel in JSON and the `Authorization` header, not cookies. |
| Forgot / reset password | **No** | No endpoint. |
| Create auction | Yes | `item {name ≤200, type ≤50, description ≤5000}`, `starting_price`, `min_increment` (0 means 1), `starts_at` (future, within 90 days), `ends_at` (1 minute to 30 days after start). |
| Edit auction | **No** | |
| Cancel auction | Yes | Owner only, only while `NOT_ACTIVE`. |
| Auction states | Yes | `NOT_ACTIVE → ACTIVE → COMPLETED`, or `NOT_ACTIVE → CANCELLED`. A worker flips status on a 1 second tick. |
| List auctions | Yes | Newest first only. Filters: `status`, `owner=me`. Keyset pagination with `next_cursor`, `limit` up to 100. |
| Sorting, price/year/make filters | **No** | |
| Search | Yes | `q` (phrases and `-exclusions`), optional `status`, cursor pagination, `highlight` snippet with `<em>`. Searches name, type and description, typo tolerant when Elasticsearch is up, PostgreSQL fallback otherwise. |
| Suggest | Yes | Type-ahead over active auctions, `q` at least 2 characters, up to 10 results `{id, title}`. |
| Trending | Yes | Active auctions with the most bids in the last hour. |
| Place bid | Yes | `POST /v1/auctions/{id}/bids {amount}` with `Idempotency-Key`. Minimum is `starting_price` (at least 1) for the first bid, otherwise `current_bid + min_increment`. Sellers cannot bid on their own auction. Rate limit 5 per second, burst 10, per user. |
| Async bid | Yes, optional | `Prefer: respond-async` returns 202 and a bid request to poll. Only when Kafka is configured. |
| Anti-sniping extension | Yes | A bid in the last 2 minutes moves the end to now + 2 minutes, at most 10 times per auction. Response carries `extended`; auction carries `extensions`. |
| Bid history | Yes | Newest first, cursor pagination. Each bid has `user_id`, `amount`, `created_at`. No bidder names. |
| Live updates | Yes | `GET /v1/ws`, public and read-only. Subscribe per auction (20 per connection). Each message is a full snapshot with `version`, `min_next_bid`, `ends_at`, `extensions`, `status`, `current_bidder_id`. |
| Wallet | Yes | `available`, `reserved`, `total`; test deposits (idempotency key required); ledger with `DEPOSIT / RESERVE / RELEASE / SETTLE` and `auction_id`. A bid reserves funds, being outbid releases them. |
| Settlement | Yes | After completion the winner's reservation is paid to the seller; `settled_at` is set. |
| Photos | **No** | No image storage, field or upload. |
| Vehicle fields (year, make, model, mileage, location, transmission, fuel, body) | **No** | Only `item.name`, `item.type`, `item.description`. |
| Reserve price | **No** | Every auction sells to the highest bid at or above `starting_price`. |
| Maximum (proxy) bid | **No** | A bid is exactly the amount sent. |
| Watchlist | **No** | |
| "Auctions I bid on" | **No** | Derivable from the wallet ledger (see 4.7). |
| Seller profile, bidder names | **No** | Only user IDs. `User` is `{id, email, activated_at}`. |
| Documents, comments, share counts | **No** | |

Everything marked **No** is either designed as a clearly labelled future state or replaced with something the backend can support. Section 13 lists the backend changes in priority order.

---

## 1. Design principles

1. **The lot is the page.** Photography takes the largest area on every screen where a car appears. Chrome is quiet, flat and thin so it never competes.
2. **Four numbers are never ambiguous.** Current bid, your bid, minimum next bid, time remaining. Each has a fixed position, a plain-language label and one typographic treatment everywhere it appears.
3. **Red means "this needs you now."** One red, used only for live, ending soon, outbid and errors. It is never decoration and never the primary button. That is how a "restrained automotive red" stays meaningful in a product where red also has to signal trouble.
4. **Server-confirmed money.** Nothing that moves money is shown as done until the server says so. Bids are pending, then accepted or rejected.
5. **Density with order.** Catalogue density (many lots, many facts per lot) organised by alignment, rules and a strict type scale, rather than by cards, shadows and padding.
6. **One system, two atmospheres.** Classic and modern lots share every component, token name and layout. Only the stage around the photograph changes (section 5.8).
7. **Fast feels premium.** No scroll-driven storytelling, no entrance animation on content, no layout shift. Motion only answers a change in auction state or a user action.
8. **Honest about the system.** Connection state, pending state and failure are shown plainly. A bidder trusts a screen that admits it is reconnecting.

---

## 2. Reference analysis

Based on the written description of the references in the brief (see section 0). Each row is a concrete rule, not "use as inspiration".

### 2.1 Vintage and classic references

| Observation in the brief | Extracted principle | Where it lands |
|---|---|---|
| Warm photography, car as focus | Warm neutral ground with no competing colour. Photo occupies at least 60% of card area and the full first screen of the lot page on desktop. | Paper atmosphere, card 4:3 image |
| Editorial, collector-magazine typography | A serif with real character for lot titles only. Everything functional stays sans. | Fraunces for titles, Archivo for UI |
| Cream, ivory, muted red, black | Two neutrals and one accent. Red is spent on state, not on brand flourishes. | Tokens in 5.1 |
| Nostalgic but not cheesy | No textures, no faux paper grain, no badges or ribbons, no script fonts. Nostalgia comes from the photo and the serif. | 5.8 |
| Dense but organised listings | Fixed grid, hairline dividers between facts, consistent line order on every card, tabular figures so prices align down a column. | Card spec 4.3 |
| Category navigation, vehicle cards, status, bid, specs, ending indicators | A horizontal category strip above the grid; status expressed as text plus a dot; the time-remaining figure changes weight and colour as the end approaches. | 4.2, 4.3 |

### 2.2 Modern and exotic references

| Observation in the brief | Extracted principle | Where it lands |
|---|---|---|
| Black backgrounds, high contrast | A true dark ground under the photo so paint and light do the work. Text limited to two weights and two sizes over imagery. | Carbon atmosphere |
| Large dramatic photography, cinematic | Wider crop for the lot stage (21:9 on desktop), no border, no radius, image bleeds to the viewport edge. | Lot stage 4.4 |
| Minimal typography, precision | Wider sans for the title, tighter tracking on large sizes, fewer words. | Archivo expanded width |
| Red used carefully | Same red token, lightened for dark ground, same meanings. | 5.1 |
| Speed | Expressed by responsiveness, not animation. | Section 9 |

### 2.3 How the two coexist

- The **photograph and its stage** change. The **instrument** (bid panel, countdown, history, specs, navigation, cards in a grid) does not.
- Grids mix classic and modern lots freely. Cards are identical; only the photo differs. A mixed grid must look like one catalogue.
- The lot page takes the atmosphere of its lot: classic lots get the paper stage and the serif title; modern lots get the carbon stage and the expanded sans title. Body, specs and the bid panel below the stage follow the site theme.
- Not copied from any reference: no existing marketplace's layout, logo, colour values or wording is reproduced.

---

## 3. Information architecture

### 3.1 Site map

```
/                       Discover
/auctions               Browse (status, category, sort)
/search?q=              Search results
/auctions/:id           Lot (detail + live bidding)
/sell                   Create auction (guided)
/account                Account home → redirects to /account/bids
/account/bids           Active bids, outbid, won
/account/watching       Watchlist
/account/listings       My listings (scheduled, live, sold, cancelled)
/account/wallet         Balance, add funds, ledger
/account/settings       Email, theme, sign out
/login  /register  /check-email  /activate?token=
```

Every screen is a URL. Filters, sort, search query and tab are in the query string so back, refresh and share work (`deep-linking`, `state-preservation`).

### 3.2 Navigation

Desktop (1024 and up), one bar, 64px:

```
[Wordmark]  Auctions  Sell        [ Search lots…                ]   Wallet ₹4,20,000   [Account ▾]
```

- Left: wordmark, two links. Centre: search with type-ahead. Right: available balance (links to wallet), account menu. Signed out: "Sign in" and "Register".
- No mega menu, no icons-only items, no notification bell (there is no notification API).
- Categories are not in the top bar. They live as a strip on Discover and Browse.

Mobile (below 768): top bar with wordmark, search icon and account; bottom tab bar with four labelled items: Discover, Browse, Bids, Account. On the lot page the bottom tab bar is replaced by the bid bar (4.4), never stacked with it.

Footer: one row. How bidding works, fees and funds (test funds notice), API status. No sitemap columns.

### 3.3 Data model the UI works with

The backend exposes `Auction { id, owner_id, item{ id, name, type, description }, starting_price, min_increment, current_bid, current_bidder_id, bid_count, extensions, settled_at, starts_at, ends_at, status, created_at, updated_at }`.

The UI needs a `Lot` view model derived from it:

```
Lot
  id, shortRef            first 8 characters of the id, shown as "Lot 7f3c2a1b"
  title                   item.name
  era                     "classic" | "modern", from item.type
  specs                   parsed from the description header (interim, see below)
  body                    description without the header
  photos[]                from the photo manifest (interim) or the API (future)
  phase                   scheduled | live | endingSoon | finalMinutes | closing | sold | unsold | cancelled
  currentBid, bidCount, minNextBid, endsAt, startsAt, extensions
  viewer                  none | leading | outbid | seller | won | lost
```

`phase` derivation (client clock corrected by server data whenever a response or snapshot arrives):

| Phase | Rule |
|---|---|
| scheduled | `NOT_ACTIVE` |
| live | `ACTIVE`, more than 1 hour left |
| endingSoon | `ACTIVE`, 1 hour or less |
| finalMinutes | `ACTIVE`, 2 minutes or less (the backend's snipe window) |
| closing | `ACTIVE` but `now ≥ ends_at`: bids are already refused, status flips within about a second |
| sold | `COMPLETED` with `current_bidder_id` |
| unsold | `COMPLETED` without a bidder |
| cancelled | `CANCELLED` |

`minNextBid` = snapshot `min_next_bid` when present, otherwise `current_bid == null ? max(starting_price, 1) : current_bid + min_increment` (the same rule as `bidding.MinimumBid`).

`viewer` derivation: `seller` if `owner_id` is me; `leading` if `current_bidder_id` is me; `outbid` if I have placed a bid on this lot (known from this session, bid history or ledger) and am not leading; `won` / `lost` once completed.

**Vehicle data, interim convention (no backend change).** Until the backend has structured fields:

- `item.name`: `"1970 Porsche 911 S 2.2 Coupé"`. Year is parsed from a leading four-digit number.
- `item.type`: `classic` or `modern`. This drives the category strip and the atmosphere. It is searchable (weighted B in PostgreSQL, ×2 in Elasticsearch).
- `item.description`: optional header of `Key: value` lines, a blank line, then prose.

```
Make: Porsche
Model: 911 S
Year: 1970
Mileage: 84,200 km
Transmission: 5-speed manual
Fuel: Petrol
Body: Coupé
Location: Pune, Maharashtra

Matching-numbers example finished in Tangerine…
```

The parser is tolerant: unknown keys are shown as given, missing keys are omitted, a description with no header is all prose. The Sell flow writes this header from form fields so sellers never type it by hand.

**Photos, interim.** A static manifest in the frontend (`/public/lots/manifest.json`, keyed by auction id or by a slug in the header: `Photos: porsche-911s-1970`) with pre-optimised files. Lots without photos get the typographic placeholder (5.9). This is demo-grade and is flagged in section 13 as the first backend dependency.

---

## 4. Page-by-page UX

Conventions for all wireframes: `▓` photo, `—` hairline rule, numbers are tabular.

### 4.1 Discover (`/`)

- **Purpose.** Put live lots in front of the visitor immediately.
- **User goal.** See what is worth looking at right now and open it.
- **Layout (desktop, 12 columns, 1320 max width).**

```
┌ nav ──────────────────────────────────────────────────────────────────────────┐
├───────────────────────────────────────────────┬───────────────────────────────┤
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ │ Ending soon                   │
│ ▓▓▓▓▓▓▓▓▓▓▓  featured lot  ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ │ ▓▓ 1984 Nissan Sunny   04:12  │
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ │    ₹3,10,000 · 14 bids        │
│ 1970 Porsche 911 S 2.2 Coupé                  │ — — — — — — — — — — — — — — — │
│ Current bid ₹42,00,000   31 bids   2d 4h left │ ▓▓ 2019 McLaren 720S   18:40  │
│                                               │ … 5 rows                      │
├───────────────────────────────────────────────┴───────────────────────────────┤
│ All   Classic   Modern          (category strip, links to /auctions?type=)    │
├───────────────────────────────────────────────────────────────────────────────┤
│ Most active now                                         View all live auctions │
│ [card] [card] [card] [card]                                                    │
│ Newly listed                                                                   │
│ [card] [card] [card] [card]                                                    │
│ Starting soon                                                                  │
│ [card] [card] [card] [card]                                                    │
│ Recently sold                                                                  │
│ [card] [card] [card] [card]                                                    │
└───────────────────────────────────────────────────────────────────────────────┘
```

  The featured block is 8 columns and capped at 56vh so the "Ending soon" list and the top of the first card row are visible on a 1366×768 screen. There is no marketing headline, no tagline and no call-to-action banner. The featured lot's own title is the largest text on the page.

- **Information hierarchy.** Featured photo, featured title and its three figures, ending-soon list, categories, card rows.
- **Data sources.**

| Section | Source | Note |
|---|---|---|
| Featured | First item of `GET /v1/auctions/trending`; fallback: first `ACTIVE` | |
| Ending soon | `GET /v1/auctions?status=ACTIVE&limit=100`, sorted by `ends_at` on the client | Interim, see 13 |
| Most active now | `GET /v1/auctions/trending?limit=8` | |
| Newly listed | `GET /v1/auctions?status=ACTIVE&limit=8` (already newest first) | |
| Starting soon | `GET /v1/auctions?status=NOT_ACTIVE&limit=8` | |
| Recently sold | `GET /v1/auctions?status=COMPLETED&limit=8` | |

  "No reserve" and "Popular" rows from the brief are dropped: reserve does not exist, and "popular" is the trending row.
- **Components.** `LotStage` (compact variant), `EndingSoonList`, `CategoryStrip`, `LotRow`, `LotCard`.
- **Interactions.** Everything is a link to a lot or to Browse. The featured lot and ending-soon list are subscribed to the WebSocket (at most 6 subscriptions on this page); card rows are not live, they refresh on focus and every 30 seconds.
- **States.** Loading: skeletons with the exact geometry of each block. Empty section: the section is omitted, except when there are no live auctions at all, where the page shows "No auctions are live right now" with the "Starting soon" row promoted to the top and a link to Sell. Error: inline "Could not load auctions. Try again" with a retry button in place of the section.
- **Desktop.** As drawn.
- **Tablet.** Featured full width at 16:9, ending-soon becomes a horizontal row of 3 compact items beneath it, card rows 3 across.
- **Mobile.** Featured lot at 4:3 full bleed, figures beneath. Ending soon as a vertical list of 3 with "See all". The category strip has three items and fits without scrolling. Card rows become single-column lists of 4 with "See all" links, not carousels (`gesture-conflicts`).

### 4.2 Browse (`/auctions`) and Search results (`/search`)

One page component, two entry points. Search adds a query; everything else is identical.

- **Purpose.** Scan many lots quickly and narrow them.
- **User goal.** Find lots worth opening.
- **Layout (desktop).**

```
┌ nav ──────────────────────────────────────────────────────────────────────────┐
│ Auctions                                                                       │
│ All   Classic   Modern                                  84 lots   Sort: Newest ▾│
│ Live   Starting soon   Sold                (status tabs, one always selected)  │
├──────────────┬────────────────────────────────────────────────────────────────┤
│ Filters      │ [card] [card] [card]                                           │
│ Status       │ [card] [card] [card]                                           │
│ Category     │ [card] [card] [card]                                           │
│ — future —   │                                                                │
│ Make         │                     Load more                                  │
│ Year         │                                                                │
│ Price        │                                                                │
└──────────────┴────────────────────────────────────────────────────────────────┘
```

- **What is real today versus planned.**

| Control | Phase 1 (works now) | Needs backend |
|---|---|---|
| Search box with type-ahead | `suggest` after 2 characters, debounced 200 ms; Enter goes to `/search?q=` | |
| Status | `status` param: Live, Starting soon, Sold. Cancelled only under "My listings". | |
| Category (Classic, Modern) | Search with the category word added to `q` when a query exists; otherwise client filter on `item.type` over loaded pages | `type` param on list and search |
| Sort: Newest | Native order | |
| Sort: Ending soon, Most bids, Price | Client sort over the first 100 active lots, labelled "of the first 100" when more exist | `sort` param |
| Make, Model, Year, Price range, Body, Location, Transmission, Fuel | **Not shown in phase 1.** The filter rail shows only Status and Category. | Structured fields + filter params |
| Reserve / no reserve | Not shown | Reserve price |

  The rail is designed for the full filter set so adding filters later needs no layout change: each group is a collapsible fieldset with a count of selected values; active filters appear as removable chips above the grid with "Clear all".
- **Pagination.** Cursor based, so "Load more" (button, 24 per page), not numbered pages and not infinite scroll. The button keeps focus position and announces "24 more lots loaded". Scroll position and loaded pages are restored on back.
- **Search results specifics.** Heading: `Results for "porsche 911"` with the count unknown (the API returns no total), so show "Showing 20" and "Load more" while `next_cursor` exists. Each card shows the API's `highlight` snippet under the title, with `<em>` rendered as a background tint, not italics, after sanitising to allow only `em`. If `backend` is `postgres`, nothing is shown to the user; it is only logged.
- **Components.** `SearchBar`, `CategoryStrip`, `StatusTabs`, `SortSelect`, `FilterRail` / `FilterSheet`, `ActiveFilterChips`, `LotGrid`, `LotCard`, `LoadMore`, `EmptyState`.
- **States.**
  - Loading: 9 card skeletons; rail and header render immediately.
  - Empty (no lots for filters): "No lots match these filters" with "Clear filters".
  - Empty (search): `No lots match "…"`, a hint ("Try the make or the year, for example 911 or 1970") and a link to all live auctions.
  - Error: "Search is unavailable right now. Try again" with retry. 400 for a query under the minimum length is prevented in the UI.
  - Rate limited (429): "Too many requests. Trying again in N seconds" using `Retry-After`, then automatic retry once.
- **Desktop.** Rail 264px sticky under the nav, grid 3 columns (4 at 1440 and up when the rail is collapsed).
- **Tablet.** Rail collapses to a "Filters" button opening a side sheet; grid 2 columns at 768, 3 at 1024.
- **Mobile.** Sticky sub-bar under the top bar: "Filters (2)" and "Sort" as two full-width buttons. Filters open as a full-height bottom sheet with a sticky "Show 31 lots" button and "Clear"; closing by swipe or the close button. Grid becomes a single column of horizontal list cards (4.3).

### 4.3 Lot card

One component, two layouts. No shadow, no radius above 2px, no hover lift.

Grid layout (desktop, tablet):

```
┌──────────────────────────────┐
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│  4:3 photo, object-fit cover
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│  top-left: status tag only when not plain "live"
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓♡│  bottom-right: watch button (44px target)
├──────────────────────────────┤
│ 1970 Porsche 911 S 2.2 Coupé │  title, serif, 2 lines max
│ 84,200 km   Manual   Pune    │  three facts, condensed sans, muted
├──────────────────────────────┤
│ Current bid        Time left │  labels, 12px
│ ₹42,00,000          2d 4h    │  figures, 20px semibold, tabular
│ 31 bids                      │  12px muted
└──────────────────────────────┘
```

List layout (mobile browse, ending-soon list, account lists): photo 112×84 on the left, title and one fact line, then bid and time on one row.

**Hierarchy, in reading order:** photo, title, current bid, time left, bid count, facts. Status sits on the photo because it changes how every other number is read.

**Content rules by phase.**

| Phase | Left figure | Right figure | Tag on photo |
|---|---|---|---|
| scheduled | "Starting bid ₹…" | "Starts 4 Oct, 18:00" | Starting soon |
| live | "Current bid ₹…" (or "Starting bid" when there are no bids) | "2d 4h" | none |
| endingSoon | same | "42m 10s" in red, with a red dot | Ending soon |
| finalMinutes | same | "01:47" in red, ticking | Final minutes |
| closing | same | "Closing" | Closing |
| sold | "Sold for ₹…" | "3 Oct" | Sold |
| unsold | "No bids" | "Ended 3 Oct" | Ended |
| cancelled | "Starting bid ₹…" | "Cancelled" | Cancelled |

Viewer overlays (signed in): a 2px left edge on the text block plus a text line replacing the bid count: "You are leading" (green) or "You have been outbid" (red). Colour is never the only signal.

Reserve status is not on the card (it does not exist). Location and the three facts appear only when the description header provides them; otherwise the line is omitted and the card keeps its height through a reserved line box.

The whole card is one link (title is the link, stretched over the card); the watch button is a separate control above it.

### 4.4 Lot page (`/auctions/:id`)

The most important screen. It is both the detail page and the live bidding experience.

- **Purpose.** Let someone judge the car and bid with confidence.
- **User goal.** Understand the lot, know exactly where the auction stands, place a bid, know whether it worked.
- **Layout (desktop, 1024 and up).**

```
┌ nav ──────────────────────────────────────────────────────────────────────────┐
│ Auctions / Classic / Lot 7f3c2a1b                                              │
├───────────────────────────────────────────────────────────────────────────────┤
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  stage: lead photo, full bleed  ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  1 / 24  ▓▓▓▓│
│ [thumb][thumb][thumb][thumb][thumb][thumb][thumb]  View all 24 photos          │
├──────────────────────────────────────────────────┬────────────────────────────┤
│ 1970 Porsche 911 S 2.2 Coupé            ♡ Watch  │ ● Live          Lot 7f3c2a1b│
│ 84,200 km   Manual   Petrol   Pune       Share   │ Time remaining              │
│ ———————————————————————————————————————————————— │ 2d 04h 12m 08s              │
│ Specifications                                   │ Ends Sun 4 Oct, 18:00 IST   │
│ Make        Porsche     Transmission  5-sp manual│ ——————————————————————————  │
│ Model       911 S       Fuel          Petrol     │ Current bid                 │
│ Year        1970        Body          Coupé      │ ₹42,00,000                  │
│ Mileage     84,200 km   Location      Pune       │ 31 bids                     │
│ ———————————————————————————————————————————————— │ ——————————————————————————  │
│ Description                                      │ Your bid                    │
│ Matching-numbers example finished in…            │ [ ₹ 42,50,000            ]  │
│                                                  │ Minimum next bid ₹42,50,000 │
│ ———————————————————————————————————————————————— │ [+₹50,000] [+₹1,00,000] [+₹2,50,000]│
│ Bid history                              31 bids │ [      Place bid ₹42,50,000     ]│
│ ₹42,00,000   Bidder a41c        2 min ago        │ Available ₹60,00,000  Add funds │
│ ₹41,50,000   You                5 min ago        │ ——————————————————————————  │
│ ₹41,00,000   Bidder a41c        9 min ago        │ Extensions used 0 of 10     │
│ Show earlier bids                                │ Bids in the last 2 minutes  │
│ ———————————————————————————————————————————————— │ extend the auction.         │
│ Seller                                           │                             │
│ Seller 9c1e   Listed 28 Sep                      │                             │
│ ———————————————————————————————————————————————— │                             │
│ More classic lots   [card] [card] [card]         │                             │
└──────────────────────────────────────────────────┴────────────────────────────┘
```

  Columns: content 7 of 12, bid panel 4 of 12 with a 1 column gap. The bid panel is sticky below the nav from the moment the stage scrolls away and is fully visible without scrolling at 1366×768 (stage capped at 52vh so the panel's first three blocks are above the fold).

- **Information hierarchy.**
  1. Lead photo.
  2. Title.
  3. Bid panel: status, time remaining, current bid, your position, bid input, minimum, action.
  4. Key facts.
  5. Specifications, description, bid history, seller, related lots.

- **Title block.** Title uses the era treatment (5.8). Year, make and model are in the title; they are not repeated as separate fields above it. The fact line shows up to four facts from the description header.

- **Photo gallery.** Stage shows the lead photo. Thumbnail strip beneath (7 visible at desktop). Click on stage or "View all" opens a full-screen viewer: one image at a time, arrow keys and swipe, counter, close on Escape, focus trapped and returned. No autoplay, no zoom-on-hover lens, no 360 spin. With no photos, the stage becomes the typographic placeholder at reduced height (32vh) and the thumbnail strip is omitted.

- **Bid panel.** Described in 4.5.

- **Bid history.** `GET /v1/auctions/{id}/bids`, 20 newest, "Show earlier bids" loads the next page. Columns: amount, bidder, time. Bidder is "You" for the signed-in user, otherwise "Bidder" plus the first 4 characters of `user_id`, stable per auction page. Extended bids cannot be identified from the bid list, so no marker is shown. New bids arriving live are inserted at the top (9.1). The list is a table with a caption.

- **Seller.** Only `owner_id` and `created_at` exist: "Seller 9c1e" and "Listed 28 Sep". If the viewer is the seller: "This is your listing" with "Cancel auction" while it is scheduled. No ratings, no avatar, no contact button.

- **Watch.** Toggles the local watchlist (4.8). Label changes between "Watch" and "Watching".

- **Share.** Uses the Web Share API when available, otherwise copies the URL and shows "Link copied".

- **Documents / history.** Not supported; the section is absent, not shown empty.

- **Related lots.** Up to 3 active lots of the same `item.type`, from the list endpoint filtered on the client. Omitted when there are none.

- **Not found / cancelled.** 404: "This lot does not exist" with a link to live auctions. Cancelled: full page still renders, bid panel replaced by "This auction was cancelled before it started."

- **Tablet (768 to 1023).** Stage 16:9. Single column. Bid panel becomes a full-width block directly under the title, and a compact sticky bar appears at the bottom once that block scrolls out of view.

- **Mobile (below 768).**

```
┌──────────────────────────┐
│ ‹ Auctions        ♡  ⤴  │
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│ swipeable gallery, 4:3, dots + "1 / 24"
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│
│ 1970 Porsche 911 S 2.2   │
│ 84,200 km  Manual  Pune  │
│ ———————————————————————— │
│ ● Live                   │
│ Time remaining  2d 4h 12m│
│ Current bid   ₹42,00,000 │
│ 31 bids                  │
│ ———————————————————————— │
│ Specifications ▾         │ collapsed sections: specs open,
│ Description ▾            │ description clamped to 6 lines,
│ Bid history ▾            │ history shows 5
│ Seller                   │
├──────────────────────────┤
│ ₹42,00,000    2d 4h 12m  │ sticky bid bar, 64px + safe area
│ [   Place bid          ] │
└──────────────────────────┘
```

  Tapping "Place bid" opens the bid sheet: a bottom sheet containing the full bid panel (time, current bid, your position, input with numeric keypad, quick increments, confirm button, balance). The sheet stays open after a bid so the result is seen in place. The sticky bar always shows current bid and time; its button label changes with state ("Place bid", "Bid again", "Sign in to bid", "Auction ended"). In landscape the bar collapses to a single 48px row.

### 4.5 Live bidding: the bid panel

This panel is the product's signature element and the one place the design spends its boldness: a calm instrument where the four numbers have fixed positions and nothing else moves.

**Fixed block order, top to bottom:** status line, time remaining, current bid, your position, bid entry, action, funds, extension meter. Blocks are separated by hairlines. Blocks may be absent (for example bid entry after the end), never reordered.

**The four numbers.**

| Label (exact wording) | Value | Treatment |
|---|---|---|
| Time remaining | Countdown | 28px semibold, tabular. Red in endingSoon and finalMinutes. Below it: "Ends Sun 4 Oct, 18:00 IST" in the viewer's time zone. |
| Current bid | `current_bid`, or "Starting bid" with `starting_price` when there are no bids | 40px semibold, tabular: the largest text in the panel |
| Your bid | The viewer's leading amount, with "You are leading" or "You have been outbid" | 16px, with icon and colour. Absent if the viewer has not bid. |
| Minimum next bid | `minNextBid` | 14px, directly under the input it constrains |

"Your max bid" from the brief is not shown: the backend has no proxy bidding, so a bid is exactly the amount entered. The label "Your bid" is used. If proxy bidding is added, a second line "Your maximum" slots under "Your bid" with no other change.

**Bid entry.**

- A labelled number field ("Your bid"), prefilled with the minimum next bid, `inputmode="numeric"`, formatted with separators on blur, digits only while typing.
- Three quick-add buttons: +1, +2 and +5 increments (`min_increment` multiples), which set the field value. They never submit.
- The action button always states the amount: "Place bid ₹42,50,000". It is disabled only while a bid is pending.
- **Confirmation step.** Pressing the button opens an inline confirmation in place of the entry block (not a modal): "Bid ₹42,50,000 on 1970 Porsche 911 S? This amount is reserved from your wallet until you are outbid." with "Confirm bid" and "Change". In finalMinutes the confirmation step is skipped after the viewer's first confirmed bid on this lot in this session, and the button label becomes "Bid now ₹…", because seconds matter. Enter in the field triggers the same path as the button.
- Validation before sending: amount is a whole number, at least the minimum next bid, at most available balance (plus the viewer's own current reservation on this lot when raising their own bid, since the engine only reserves the difference). Errors appear under the field and are linked with `aria-describedby`.

**Request and response handling.** One `Idempotency-Key` (UUID) is generated per confirmed intent and reused on every retry of that intent.

| Response | UI |
|---|---|
| pending | Button shows "Placing bid…" with a spinner; field read-only; no other number changes. No optimistic price change. |
| 201 accepted | Panel updates from the response (`current_bid`, `bid_count`, `ends_at`). "Bid accepted. You are leading at ₹42,50,000." in green in the "Your bid" block; the row appears in history. If `extended` is true: "Your bid extended the auction to 18:02:10." |
| 200 replay | Treated as accepted, shown once. |
| 422 too low | "Someone bid first. The minimum is now ₹43,00,000." Field is set to `minimum_amount`, focus returns to the field, the confirmation step is skipped for the immediate retry. |
| 422 insufficient funds | "Not enough available funds. You have ₹40,00,000 available." with "Add funds" opening the wallet deposit sheet in place; the entered amount is preserved. |
| 409 not open / ended | "This auction has ended." or "This auction has not started." Panel switches to the matching phase after refetching. |
| 403 own auction | Never reachable in normal use: sellers see no bid entry, only "You are the seller of this lot." |
| 401 | One silent token refresh and retry with the same key. If that fails, sign-in sheet opens with the amount preserved (4.9). |
| 429 | "You are bidding too fast. Try again in N seconds." Button disabled with a countdown from `Retry-After`. |
| 503 | Automatic retry after `Retry-After` (1 second), up to 3 times with the same key, showing "Auction is busy, retrying…". Then "Could not place your bid. Try again." with the button enabled. |
| Network failure | "No response from the server. Your bid may or may not have been placed." with "Check and retry", which resends with the same key (safe by idempotency). |
| 202 (only if async mode is enabled later) | "Bid received, confirming…" then resolved from the WebSocket or by polling `status_url`: accepted, rejected (with reason and `minimum_amount`), or failed. |

**Live updates.**

- On mount: fetch the auction and first page of bids, then subscribe on the WebSocket. Apply a snapshot only if `version` is higher than the one held. REST responses are applied through the same reducer (bid responses are authoritative for the fields they carry).
- On `cause: "bid.placed"` from someone else: current bid, bid count, minimum next bid and end time update; the newest bid row is fetched (first page refetch, throttled to once per second). If the field value is now below the minimum, it is raised to the minimum and a note appears: "Minimum raised to ₹43,00,000."
- **Outbid** (I was `current_bidder_id`, now someone else is): the "Your bid" block turns red with an icon: "You have been outbid. Current bid ₹43,00,000." The action button reads "Bid again ₹43,50,000". A toast is also shown if the panel is scrolled out of view, and the document title is prefixed with "Outbid".
- **Extension** (`extensions` increased): the time block shows "Extended" beside the countdown for 6 seconds and the extension meter fills one segment. The countdown value jumps to the new time without animation.
- **Extension meter.** Ten small segments labelled "Extensions used 3 of 10", with one line of explanation. It only becomes prominent in endingSoon and finalMinutes; before that it is a single muted line. At 10 of 10: "No further extensions. The auction ends when the timer reaches zero."
- `auction.completed`: panel switches to the ended layout (below). `auction.settled`: adds "Payment settled" for winner and seller. `auction.cancelled`, `auction.activated`: phase changes accordingly.

**Countdown.**

- Format: more than 24 hours "2d 04h 12m"; under 24 hours "04h 12m 08s"; under 1 hour "12:08"; ticks once per second, aligned to the second.
- Driven by `ends_at`, never by decrementing a counter. When the tab becomes visible again it recalculates immediately.
- At zero while status is still `ACTIVE`: "Closing…" and bid entry is disabled (the engine already refuses bids). Resolves when the completed snapshot arrives, or by refetch after 3 seconds.
- Clock skew: there is no server time endpoint and the `Date` header is not exposed through CORS. Phase 1 trusts the client clock and treats server responses as the truth for whether a bid is allowed. See 13.

**Connection states** (a single line at the top of the panel, next to the status):

| State | Shown as | Behaviour |
|---|---|---|
| Live | Red dot, "Live" | |
| Connecting (first load) | Grey dot, "Connecting" | Data from REST is already shown |
| Reconnecting | Amber dot, "Reconnecting… prices may be out of date" | Backoff 0.5 s doubling to 10 s with jitter; REST polling every 5 s meanwhile; bidding stays enabled because the server validates |
| Offline (`navigator.onLine` false) | Amber banner across the page: "You are offline" | Bid button disabled with the reason beside it |
| Restored | "Back online" for 3 seconds | Refetch auction and bids, resubscribe |

Close codes: 1001 reconnect immediately; 1013 (at capacity) retry after 10 s and fall back to polling; 1008 (too slow) reconnect and drop non-essential subscriptions; 1009 is a client bug, log it.

**Ended layouts.**

| Viewer | Panel content |
|---|---|
| Winner | "You won this auction" in green. "Winning bid ₹43,50,000". "Paid from your reserved funds" once `settled_at` is set, otherwise "Settling payment…". Link to wallet. |
| Outbid participant | "Auction ended. You did not win." "Sold for ₹43,50,000". "Your reserved funds were released." Link to related lots. |
| Seller | "Sold for ₹43,50,000" with settlement status, or "Ended with no bids". |
| Anyone else | "Sold for ₹43,50,000" and "31 bids", or "Ended with no bids". |

"Auction just ended" is the first 10 seconds of the above, with a single line "This auction has just ended" above the result; no confetti, no modal.

**Signed-out and not-ready states.**

| Situation | Panel |
|---|---|
| Signed out | Everything visible and live. Entry replaced by "Sign in to bid" (primary) and "Create an account". Returns to this lot after sign-in. |
| Signed in, zero available funds | Entry visible; under it "Add funds to bid" with a link. |
| Scheduled | "Bidding opens Sun 4 Oct, 18:00 IST" with a countdown to start, starting bid shown, Watch as the primary action. |
| Seller | No entry. "You are the seller of this lot." Cancel available while scheduled. |

**Reserve states.** Reserve met, reserve not met and no reserve are **not implemented** because the backend has no reserve price. The status line reserves a slot to the right of "Live" for a reserve tag so adding it later needs no layout change.

### 4.6 Sell (`/sell`)

Supported by the backend (`POST /v1/auctions`), so it is designed, as a guided flow of five steps. It is a real sequence, so numbered steps are appropriate.

```
1 Vehicle   2 Details   3 Description   4 Auction   5 Review
```

| Step | Fields | Maps to |
|---|---|---|
| 1 Vehicle | Year, make, model, variant (optional), era (Classic / Modern, preselected from year: before 1990 is classic) | `item.name` (composed, with a live preview and a 200 character count), `item.type` |
| 2 Details | Mileage and unit, transmission, fuel, body type, location | Description header |
| 3 Description | Free text | `item.description` (header + text, counter against 5000 total) |
| 4 Auction | Starting bid, minimum increment (default suggested as about 1% of the starting bid, rounded), start date and time, end date and time or a duration choice (3, 5, 7, 10 days) | `starting_price`, `min_increment`, `starts_at`, `ends_at` |
| 5 Review | A real lot card and a read-only bid panel preview, a plain summary of dates in the seller's time zone, and what happens next | |

- **Photos step.** Not in phase 1 because there is no upload. The step list is designed to take "Photos" between Details and Description once storage exists. Until then, Review states: "Photos cannot be uploaded yet. Your lot will show a placeholder."
- **Rules shown as helper text, enforced before submit.** Start must be in the future and within 90 days. End must be between 1 minute and 30 days after the start. Amounts are whole numbers in the smallest currency unit handled by the money input.
- **Behaviour.** Each step validates on "Continue"; fields validate on blur. Back never loses data. The draft autosaves to `localStorage` and is restored with a notice ("Draft restored from 2 Oct"). Leaving with unsaved changes asks for confirmation.
- **Submit.** Button "Publish auction". 201: go to the new lot page with a one-time notice "Your auction is scheduled. Bidding opens Sun 4 Oct, 18:00." 400: the `fields` map from the API is routed to the right step and field (`item.name`, `starts_at`, `ends_at`…), an error summary at the top links to each one and receives focus.
- **After publishing.** The listing cannot be edited (no endpoint), which Review says plainly: "You cannot change a listing after publishing. You can cancel it before bidding opens."
- **Desktop.** Form column 640px, sticky live preview card on the right.
- **Mobile.** One step per screen, sticky "Continue" at the bottom, progress as "Step 2 of 5" text plus a thin bar, preview only on Review.
- **Signed out.** Redirect to sign in, then back to `/sell`.

### 4.7 Account (`/account/*`)

Four lists and a wallet. No charts, no statistics tiles, no activity feed.

Desktop: left list of sections (Bids, Watching, Listings, Wallet, Settings), content on the right. Mobile: the same sections as a segmented control under the page title; Account is one bottom tab.

| Section | Content | Source |
|---|---|---|
| Bids | Three groups: "Leading", "Outbid", "Ended" (won first, then lost). List-layout lot cards with the viewer's amount. | Interim: distinct `auction_id`s from `GET /v1/wallet/ledger` entries of kind `RESERVE`, then `GET /v1/auctions/{id}` for each; grouped by comparing `current_bidder_id` to me. Live lots here are subscribed on the WebSocket (up to 20). Preferred: a `bidder=me` list endpoint. |
| Watching | Section 4.8 | |
| Listings | Tabs: Scheduled, Live, Sold, Ended without bids, Cancelled. Scheduled rows have "Cancel auction" behind a confirmation dialog ("Cancel this auction? This cannot be undone."). | `GET /v1/auctions?owner=me&status=` |
| Wallet | Three figures: Available, Reserved in bids, Total. "Add funds" (amount field, clearly labelled "Test funds. No real payment is taken."). Ledger table: date, type in plain words ("Added funds", "Reserved for bid", "Released after being outbid", "Paid for won auction"), linked lot, amount with sign. "Load earlier" via `before`. | `GET /v1/wallet`, `POST /v1/wallet/deposits` with `Idempotency-Key`, `GET /v1/wallet/ledger` |
| Settings | Email (read-only), theme (System, Light, Dark), "Sign out". Sign out is separated at the bottom. | `GET /v1/users/me`, `POST /v1/auth/logout` |

"Auction history" from the brief is the "Ended" group under Bids and the Sold tab under Listings; it is not a separate page.

States: each list has loading skeleton rows, an empty state with one action ("You have not bid on anything yet" with "Browse live auctions"; "You have no listings" with "Sell a car"), and an inline error with retry.

### 4.8 Watchlist (`/account/watching`)

There is no watchlist API. Phase 1 is device-local and says so.

- Stored as a list of auction ids in `localStorage`, available signed out as well.
- Page: list-layout cards sorted by end time, live through the WebSocket (first 20), each with "Remove". Ended lots move to a collapsed "Ended" group.
- A single quiet line at the top: "Your watchlist is saved on this device."
- Empty: "You are not watching any lots" with "Browse live auctions".
- A lot that no longer exists (404) is removed silently.
- When a server watchlist exists, the local list is uploaded once on sign-in and the notice is removed.

### 4.9 Authentication

| Screen | Content and behaviour |
|---|---|
| Register | Email, password (show/hide toggle, `autocomplete="new-password"`, helper text "At least 15 characters. A passphrase works well." with a live length count, paste allowed). 201 → Check email screen. 409 → "An account with this email already exists" with a link to sign in. 400 → field errors from `fields`. 429 → wait message from `Retry-After`. |
| Check email | "We sent an activation link to a@b.com." with "Send again" (`resend-activation`, always succeeds from the user's point of view; button disabled for 60 s after use). |
| Activate | Reads `token` from the URL, calls the API. Success → "Your account is active" with "Sign in". Failure → "This link is invalid or has expired" with an email field and "Send a new link". Depends on the email linking to the frontend, see 13. |
| Login | Email, password. 401 → "Email or password is incorrect" (one message, no hint which). 403 → "Activate your account first" with "Send the link again". 429 as above. On success return to the page the user came from. |
| Forgot password | **Not built**: no endpoint. The login screen has no "Forgot password" link until the backend supports it. |

**Session.**

- Access token in memory only. Refresh token in `localStorage` (the API returns it in JSON; an httpOnly cookie would be safer, see 13).
- One refresh at a time. A 401 on any request triggers a single shared refresh, then the request is retried once. Refresh tokens rotate and reuse returns 401, so refreshes are also serialised across tabs with the Web Locks API, and new tokens are broadcast to other tabs.
- Refresh proactively about 1 minute before expiry while the tab is visible.
- If refresh fails: clear the session, keep the user on the page when it is public (the lot page stays live), and show "Your session expired. Sign in again." Protected pages redirect to sign in with a return path.
- Protected routes: `/sell`, `/account/*` (except Watching). Protected actions on public pages (bid) open a sign-in sheet in place and resume the action afterwards.
- Sign out calls `POST /v1/auth/logout` with the refresh token and clears all tabs.

---

## 5. Design system

Tokens are semantic and defined once as CSS custom properties (Tailwind 4 `@theme`). Components never use raw hex values.

### 5.1 Colour

Two themes share one set of names. "Paper" is the light theme and the default; "Carbon" is the dark theme. Contrast ratios were computed for the listed pairs.

| Token | Paper | Carbon | Use |
|---|---|---|---|
| `bg` | `#ECEBE7` | `#101113` | Page background |
| `bg-secondary` | `#E1DFD8` | `#17191C` | Bands, table header rows, skeleton base |
| `surface` | `#F6F5F1` | `#1C1F23` | Cards, panels, inputs |
| `surface-elevated` | `#FFFFFF` | `#262A2F` | Sheets, menus, dialogs, toasts |
| `text` | `#14130F` (15.6:1 on bg) | `#F2F0EB` (16.6:1) | Titles, figures, body |
| `text-secondary` | `#45433D` (8.3:1) | `#B8B5AD` (9.2:1) | Fact lines, descriptions, labels |
| `text-muted` | `#66635B` (5.0:1 on bg, 4.5:1 on bg-secondary) | `#9D9A92` (6.7:1; 5.1:1 on elevated) | Timestamps, helper text, bid counts |
| `border` | `#CBC8BE` | `#30343A` | Hairlines, dividers, card outlines (decorative, not relied on alone) |
| `border-strong` | `#7C786D` (3.7:1) | `#737882` (4.3:1) | Input and control outlines (meets 3:1 for UI components) |
| `action` | `#14130F` with text `#ECEBE7` | `#F2F0EB` with text `#101113` | Primary buttons. Ink on paper, paper on carbon. |
| `accent` / `live` | `#B0221B` (5.7:1 on bg; white on it 6.8:1) | `#F4665D` (6.2:1 on bg; 4.75:1 on elevated) | Live dot, ending soon, final minutes, outbid |
| `danger` | same as accent | same as accent | Errors and destructive actions, always with an icon and text |
| `success` | `#1D6A43` (5.5:1) | `#4FC48C` (8.6:1) | Bid accepted, leading, won |
| `warning` | `#855400` (5.4:1) | `#E3A63A` (8.8:1) | Reconnecting, extended, pending |
| `focus` | `#14130F` | `#F2F0EB` | 2px ring with 2px offset |

Rules:

- **Red is state.** It appears only for live, ending soon, final minutes, outbid, errors and destructive confirmation. Never on primary buttons, links, logos, hover states or decoration. On any screen, visible red should mean something is live or needs attention.
- **Primary action is ink.** One primary button per screen.
- **Auction-live** = red dot + "Live". **Ending soon** = red time figure + "Ending soon" tag. **Final minutes** = the same plus a 2px red top edge on the bid panel.
- Green and amber appear only as text with an icon or as a 2px edge, never as filled backgrounds larger than a tag.
- No gradients, except one functional scrim: a bottom-up `text`-to-transparent overlay behind text that sits on a photograph.
- Why not the palettes the skill database suggested: its "Auction Platform" row proposes dark mode with bid green, outbid red and countdown amber, and its "Automotive" row proposes slate plus action red. The state colours are adopted. The slate and the gold-accent luxury palette are rejected in favour of the brief's warm neutrals and the project's existing `paper` and `ink`.

### 5.2 Typography

Two families, clearly different, both variable and available from Google Fonts under open licences.

| Role | Family and settings | Sizes (desktop / mobile) |
|---|---|---|
| Display (lot title on the lot page, featured lot) | **Fraunces**, weight 400, optical size auto, `SOFT` 0, `WONK` 0, tracking −0.01em | 44 / 30, line height 1.1 |
| Heading (section headings, card titles, page titles) | Fraunces 500 | 28, 22, 18 / 24, 20, 17, line height 1.2 |
| Display, modern lots | **Archivo**, weight 600, width 112, tracking −0.02em | same sizes as display |
| Body | Archivo 400, width 100 | 16, line height 1.55; description measure 60 to 72 characters |
| Metadata (fact lines, labels, table headers, tags) | Archivo 500, width 88 | 13 and 12, line height 1.35, sentence case |
| Numeric (bids, countdowns, balances) | Archivo 600, width 100, `font-variant-numeric: tabular-nums lining-nums` | 40 (panel current bid), 28 (countdown), 20 (card figures), 16 (table) |

- Why these: Fraunces gives the collector-catalogue warmth for classic lots without the fashion-brand feel of the high-contrast serifs the skill database suggested (Playfair, Cormorant; Cormorant is also what the watch prototype used). Archivo has a width axis, which lets one family be condensed for dense metadata and widened for the badge-like titles of modern cars, so "two atmospheres" costs no extra font.
- No monospace anywhere. Tabular figures do the job without the terminal look.
- No all-caps labels, no letter-spaced eyebrows. Labels are sentence case at metadata size.
- Type scale: 12, 13, 14, 16, 18, 20, 22, 28, 36, 44. Nothing else.
- Loading: self-host both as variable WOFF2, subset to Latin plus the rupee sign, `font-display: swap`, preload the two files, fallback stacks with `size-adjust` to avoid shift (Fraunces → Georgia, Archivo → Arial).
- Currency and dates use `Intl.NumberFormat` and `Intl.DateTimeFormat`. Amounts are integers in the smallest unit; they are never parsed or stored as floats.

### 5.3 Spacing

4px base: `4, 8, 12, 16, 24, 32, 48, 64, 96`.

- Inside components: 8 and 12. Between related blocks: 16 and 24. Between sections: 48 desktop, 32 mobile. Page gutters: 32 desktop, 24 tablet, 16 mobile.
- Grid: 12 columns, 24px gutter, max width 1320. Card grid gap 24 desktop, 16 mobile.
- Density is deliberately on the tight side (the skill's density dial at 7 of 10): the brief asks for no whitespace that hides information.

### 5.4 Radius

| Value | Used for |
|---|---|
| 0 | Lot stage, full-bleed imagery, dividers |
| 2px | Cards, card photos, tags, thumbnails |
| 4px | Buttons, inputs, selects, menus |
| 8px | Sheets, dialogs, toasts |
| Full | Status dot, toggle thumb only |

No pill buttons, no pill tags.

### 5.5 Shadows

Flat by default. Hierarchy comes from borders and background steps.

| Token | Value | Used for |
|---|---|---|
| `shadow-overlay` | `0 8px 24px rgb(20 19 15 / 0.14)` (Paper), `0 8px 24px rgb(0 0 0 / 0.5)` (Carbon) | Menus, dialogs, sheets, toasts |
| `shadow-sticky` | `0 1px 0 var(--border)` | Sticky nav and sticky bid bar edge |

Cards have no shadow in any state.

### 5.6 Borders and dividers

- 1px `border` hairlines separate: blocks inside the bid panel, rows in bid history and ledger, spec rows, card photo from card text, sections on the lot page.
- Spec tables use row rules only, no vertical rules, no zebra.
- Cards have a 1px `border` outline in Paper and none in Carbon (the surface step is enough).
- A 2px coloured left or top edge is the only "accent border" and is reserved for viewer state (leading, outbid) and final minutes.

### 5.7 Icons and controls

- One icon set, 1.5px stroke (Lucide or Phosphor regular). Icons accompany text; icon-only buttons (watch on a card, close, gallery arrows) have accessible names.
- No emoji. No arrow glyphs appended to link text.
- Buttons: primary (ink fill), secondary (1px `border-strong` outline), quiet (text only, underlined on hover), destructive (red outline; red fill only inside a confirmation dialog). Heights 44 default, 36 compact on desktop tables only.
- Inputs: 44 high, visible label above, helper or error text below, 1px `border-strong`, 2px `focus` ring.

### 5.8 Atmospheres: one system, two worlds

`data-era="classic" | "modern"` is set on the lot stage (and only there). It changes exactly these things:

| Property | Classic | Modern |
|---|---|---|
| Stage background (behind and around the photo, letterbox areas) | Paper `bg-secondary` | Carbon `bg` (true dark, regardless of site theme) |
| Stage aspect ratio (desktop) | 16:9 | 21:9 |
| Title face | Fraunces 400 | Archivo 600, width 112 |
| Title case and tracking | As written, −0.01em | As written, −0.02em |
| Photo treatment | None. No filter, no vignette. | None. A bottom scrim only where text overlaps the image. |
| Gallery thumbnails | 2px radius, 8px gap | 0 radius, 4px gap |
| Rule under the title block | 1px `border` | 1px `border-strong` |

Everything else, including the bid panel, cards, specs, history, colours of state, spacing and controls, is identical. Cards in grids ignore era entirely. The site theme (Paper or Carbon) is the user's choice or system setting and is independent of era; a classic lot in Carbon theme keeps its paper stage, a modern lot in Paper theme keeps its dark stage. That contrast is the intended effect: the stage is a lit plinth, the page around it is the catalogue.

Photography direction (for seed content and seller guidance): three-quarter front as the lead image, car fills at least 70% of frame width, horizon level, no text or watermarks baked in, consistent 4:3 source crop with a 21:9-safe centre band for modern lots.

### 5.9 Placeholder for lots without photos

A flat `bg-secondary` block at the correct aspect ratio containing the year in Fraunces at large size and the make and model beneath, in `text-muted`. No stock silhouettes, no generated car images, no "image coming soon" icon. It reads as a catalogue page awaiting its plate.

### 5.10 Voice and copy

- Sentence case everywhere. Plain verbs. Buttons say what happens: "Place bid ₹42,50,000", "Publish auction", "Add funds", "Cancel auction".
- One name per thing, used everywhere: lot, bid, current bid, minimum next bid, time remaining, available, reserved, watch.
- Errors state what happened and what to do, without apology: "Someone bid first. The minimum is now ₹43,00,000."
- Empty states invite one action.
- No exclamation marks, no countdown hype ("Hurry"), no invented scarcity.

---

## 6. Component architecture

Stack (matches the existing prototypes and `CORS_ALLOWED_ORIGINS=http://localhost:5173`): Vite, React 19 with the React Compiler, TypeScript, Tailwind 4, React Router, TanStack Query for REST state, a small external store for live auction state, native `WebSocket`. Types generated from `api/openapi.json`. GSAP and three.js from the prototypes are not used. Lives in a new top-level `web/` directory.

```
web/src/
  app/            routes, layouts, providers, error boundary
  api/            client (fetch wrapper, auth, refresh, idempotency), generated types, endpoints
  realtime/       socket manager, subscription registry, live store
  domain/         lot view model, phase and viewer derivation, money, time, description parser
  features/
    discover/  browse/  lot/  bidding/  sell/  account/  wallet/  watchlist/  auth/
  ui/             design-system primitives
  styles/         tokens.css
```

### 6.1 Primitives (`ui/`)

| Component | Responsibility |
|---|---|
| `Button`, `IconButton`, `Link` | Variants from 5.7, pending state, required accessible name on icon buttons |
| `Field`, `TextInput`, `MoneyInput`, `Select`, `DateTimeInput`, `Checkbox`, `RadioGroup` | Label, helper, error wiring; `MoneyInput` works in integer minor units and formats on blur |
| `Tabs`, `SegmentedControl` | URL-driven selection |
| `Dialog`, `Sheet` | Focus trap, Escape, return focus; `Sheet` is side on desktop and bottom on mobile |
| `ConfirmationDialog` | Destructive confirmations only (cancel auction) |
| `Toast` | Polite live region, 5 s auto-dismiss, never the only place an error appears |
| `Skeleton` | Geometry-matched placeholders |
| `EmptyState`, `InlineError` | Message plus one action; error has retry |
| `Tag`, `StatusDot` | Text-first status; dot is decorative |
| `Table` | Row rules, caption, responsive collapse to stacked rows |
| `Menu` | Account menu, sort |
| `VisuallyHidden`, `SkipLink`, `LiveRegion` | Accessibility helpers |

### 6.2 Domain components

| Component | Responsibility | Notes |
|---|---|---|
| `AppShell`, `Navbar`, `BottomTabs`, `Footer` | Layout and navigation | `BottomTabs` hidden on the lot page |
| `SearchBar` | Query input, type-ahead from `suggest`, keyboard listbox | Debounced, cancels stale requests |
| `CategoryStrip` | All, Classic, Modern | Links, current item marked |
| `StatusTabs`, `SortSelect`, `FilterRail`, `FilterSheet`, `ActiveFilterChips` | Browse controls bound to the URL | Rail and sheet render the same filter definitions |
| `LotGrid`, `LotRow` | Grid and list containers, skeleton counts | |
| `LotCard` | Section 4.3; `layout="grid" | "list"` | One component; replaces the brief's separate AuctionCard and VehicleCard, which would show the same data |
| `LotPhoto` | Responsive image, aspect box, blur placeholder, typographic fallback | Used by card, stage, thumbnails |
| `LoadMore` | Cursor pagination control with announcement | Replaces Pagination: the API is cursor based |
| `LotStage` | Lead photo with era atmosphere | Sets `data-era` |
| `PhotoGallery`, `PhotoViewer` | Thumbnails and full-screen viewer | |
| `LotHeader` | Title, facts, watch, share | |
| `SpecTable` | Key and value pairs from parsed specs | Two columns desktop, one mobile |
| `LotDescription` | Prose with clamp and expand on mobile | Plain text only, rendered safely |
| `BidPanel` | Section 4.5 container; arranges blocks by phase and viewer | The only component that knows the block order |
| `PriceDisplay` | Labelled money figure, sizes `panel`, `card`, `table` | Handles "Starting bid" versus "Current bid" versus "Sold for" |
| `Countdown` | Time remaining from `endsAt`, phase-aware format and colour | Owns its own tick; nothing above it re-renders per second |
| `AuctionStatus` | Dot plus label for phase and connection | |
| `ViewerPosition` | "You are leading" / "You have been outbid" / result | |
| `BidForm` | Field, quick increments, validation, confirmation step, submit | Emits intent; does not talk to the network |
| `ExtensionMeter` | Extensions used of 10 | |
| `FundsLine` | Available balance and "Add funds" | |
| `BidBar`, `BidSheet` | Mobile sticky bar and sheet wrapping `BidPanel` | |
| `BidHistory` | Table of bids, live insertions, "Show earlier bids" | |
| `SellerCard` | Seller reference, listed date, owner actions | |
| `WatchButton` | Toggle local watchlist | |
| `RelatedLots` | Up to 3 cards | |
| `ConnectionBanner` | Offline and reconnecting messages | |
| `SellWizard`, `StepIndicator`, step forms, `ListingPreview` | Section 4.6 | |
| `AccountLayout`, `BidsList`, `ListingsList`, `WalletSummary`, `DepositForm`, `LedgerTable` | Section 4.7 | |
| `AuthForm`, `SignInSheet`, `RequireAuth` | Section 4.9 | |

### 6.3 State and data flow

- **REST state**: TanStack Query. Keys by endpoint and params. Lists are stale after 30 s and refetch on focus; a single lot is kept fresh by the live store.
- **Live store**: a map of auction id to the latest snapshot (highest `version` wins), exposed through `useSyncExternalStore` with per-field selectors (`useLotField(id, "currentBid")`) so a new bid re-renders the price, count and minimum, and nothing else.
- **Socket manager**: one connection per tab, reference-counted subscriptions, capped at 20 (the server limit), least-recently-visible evicted first; resubscribes after reconnect; pauses when the tab is hidden for more than 60 s and catches up by REST on return.
- **Bid machine** (per lot): `idle → confirming → pending → accepted | rejected(reason) | uncertain`. `uncertain` (network failure) can only go to `pending` with the same idempotency key.
- **Auth**: context with user, access token in memory, single-flight refresh.
- **Money and time** helpers are the only places that format amounts and dates.

---

## 7. Responsive behaviour

Breakpoints: 375 (base), 768, 1024, 1440. Built mobile-first.

| Element | Mobile (below 768) | Tablet (768 to 1023) | Desktop (1024 and up) |
|---|---|---|---|
| Navigation | Top bar + 4 bottom tabs; search opens full screen | Top bar with inline search, no bottom tabs | Full top bar |
| Lot cards | List layout, single column | Grid, 2 columns | Grid, 3 columns (4 at 1440 with rail collapsed) |
| Filters | Full-height bottom sheet, sticky apply button | Side sheet | Sticky rail, 264px |
| Sort | Native select in the sticky sub-bar | Menu | Menu |
| Lot stage / gallery | Swipeable 4:3, counter, tap for viewer | 16:9 stage + thumbnail strip | 16:9 or 21:9 stage + strip |
| Bid panel | Summary block inline + sticky bid bar + bid sheet | Inline block under title + sticky bar after scroll | Sticky right column |
| Countdown | In the sticky bar and inline; compact format ("2d 4h") in the bar until under 1 hour | Full format | Full format |
| Bid history | 5 rows, "Show all" expands in place; time as relative only | 10 rows | 20 rows, absolute time on hover and focus |
| Specs | One column | Two columns | Two columns |
| Sell | One step per screen, sticky continue | Form + preview stacked | Form + sticky preview |
| Account | Segmented control, lists | Left list + content | Left list + content |
| Tables (ledger) | Stacked rows: label and value pairs | Table | Table |

Mobile-specific rules: bid field uses the numeric keypad and stays visible above the keyboard (sheet resizes with `visualViewport`); the bid bar respects the bottom safe area; no horizontal scrolling anywhere except the photo gallery, which has visible previous and next buttons as an alternative to swiping; minimum body size 16px; touch targets 44px with 8px between them; `min-height: 100dvh` for full-height layouts.

---

## 8. Interaction states

### 8.1 Auction and viewer states

| State | Where it shows | Treatment |
|---|---|---|
| Auction active | Card, panel | Red dot "Live", figures in `text` |
| Ending soon (1 hour or less) | Card, panel, ending-soon list | Time in red, tag "Ending soon" |
| Final minutes (2 minutes or less) | Card, panel | Ticking seconds in red, 2px red top edge on panel, extension meter prominent |
| Extended | Panel | "Extended" beside the countdown for 6 s, meter fills, amber |
| Closing | Card, panel | "Closing…", entry disabled |
| Auction just ended | Panel | One line above the result for 10 s |
| Auction ended | Card, panel | "Sold for" or "Ended with no bids"; history stays |
| Scheduled | Card, panel | "Starting soon", countdown to start, starting bid |
| Cancelled | Card (own listings), panel | Muted, "Cancelled" |
| User has not bid | Panel | No "Your bid" block |
| User has highest bid | Card, panel, account | Green "You are leading" with check icon, 2px green edge |
| User has been outbid | Card, panel, account, toast, tab title | Red "You have been outbid" with icon, 2px red edge, button "Bid again ₹…" |
| Winning auction (ended, won) | Panel, account | Green "You won this auction", settlement line |
| Losing auction (ended, lost) | Panel, account | Neutral "You did not win", funds released line |
| Reserve met / not met / no reserve | Nowhere yet | **Future.** Slot reserved in the status line |

### 8.2 Bid states

| State | Treatment |
|---|---|
| Bid validation error | Under the field, red with icon: "Enter at least ₹42,50,000" / "Enter a whole amount" / "You have ₹40,00,000 available" |
| Confirming | Inline confirmation replaces the entry block |
| Pending | "Placing bid…", controls read-only |
| Bid accepted | Green confirmation in the position block, history row added |
| Bid rejected | Reason from the table in 4.5, field corrected where possible |
| Uncertain | "Your bid may or may not have been placed" with "Check and retry" |
| Rate limited | Disabled button with seconds remaining |
| Authentication required | "Sign in to bid"; sign-in sheet preserves the amount |

### 8.3 System states

| State | Treatment |
|---|---|
| Loading | Skeletons matching final geometry. No full-page spinners. Buttons show their own pending state. Skeletons appear only if the wait exceeds 200 ms. |
| Empty | Message and one action, per page (section 4) |
| Error | Inline, at the place the content would be, with "Try again". The error boundary page offers "Reload" and a link home. |
| Network disconnected | Page banner "You are offline", bid disabled with reason |
| Reconnecting | Amber dot and line in the panel, REST polling |
| Session expired | Notice with "Sign in again"; public content stays |
| Not found | Page-level message with a link to live auctions |

---

## 9. Motion

Tokens: `fast` 120 ms, `base` 180 ms, `slow` 240 ms; enter `cubic-bezier(0.2, 0, 0, 1)`, exit `cubic-bezier(0.4, 0, 1, 1)` at about two thirds of the enter time. Only `transform`, `opacity` and `background-color` are animated.

### 9.1 The complete list

| Trigger | Motion |
|---|---|
| New bid changes the current price | The figure's background tints with `warning` at 16% and fades over 600 ms. The number itself swaps instantly, no rolling digits. |
| New row in bid history | Row fades in over 180 ms with the same tint |
| Countdown tick | None. Digits swap. Colour changes at thresholds are instant. |
| Extension | "Extended" label fades in, holds, fades out; meter segment fills over 180 ms |
| Outbid / leading / result | Position block cross-fades over 180 ms |
| Bid pending → accepted | Button spinner, then the button returns; confirmation text fades in |
| Gallery image change | 180 ms cross-fade; swipe tracks the finger on touch |
| Sheet, dialog, menu | Slide or fade in 180 to 240 ms, out faster |
| Toast | Fade and 8px rise in, fade out |
| Hover | Card: title underline. Button: background step. 120 ms. No scaling, no lift, no image zoom. |
| Route change | None. New content renders; focus moves to the page heading. |
| Skeleton | Slow opacity pulse (1.6 s); static under reduced motion |

### 9.2 Not used

Scroll-triggered reveals, parallax, staggered grid entrances, page transitions, number-rolling tickers, pulsing "live" dots, auto-advancing carousels, cursor effects, 3D. The design-system search returned a scroll-reveal GSAP preset and a "scroll-triggered storytelling" page pattern; both are rejected for this product.

Reduced motion: all transitions become instant, the price tint becomes a 1 s static outline, skeletons stop pulsing. No state depends on an animation finishing.

---

## 10. Accessibility

Target: WCAG 2.2 AA.

- **Keyboard.** Everything operable by keyboard in visual order. Skip link to main content. Type-ahead is a combobox with listbox semantics and arrow keys. Gallery viewer: arrows, Home, End, Escape. Filters: fieldsets with legends. No keyboard traps except intentional focus traps in dialogs and sheets.
- **Focus.** 2px ring, 2px offset, 3:1 against adjacent colours, never removed. Sticky nav and bid bar never cover the focused element (`scroll-padding` set to their heights). Focus moves to the page heading on route change, into dialogs on open, back to the trigger on close, and to the error summary on a failed multi-field submit.
- **Contrast.** Ratios in 5.1. Text over photos always sits on a scrim that guarantees 4.5:1. Red, green and amber are never the only signal: each has a word and an icon.
- **Screen readers.**
  - Landmarks: header, nav, main, footer; one `h1` per page; headings in order.
  - Bid panel is a labelled region ("Bidding"). Each of the four numbers has a programmatic label.
  - Live announcements use one polite status region per page with complete phrases, throttled to one message per 5 s, the latest replacing queued ones: "Current bid ₹43,00,000, 32 bids." Assertive region only for things that concern the user directly: "You have been outbid. Current bid ₹43,00,000.", "Bid accepted. You are leading at ₹43,50,000.", "Auction ended. You won."
  - The countdown is not a live region. It exposes a static accessible name ("Time remaining: 2 days 4 hours") updated each minute, and announces thresholds once: "1 hour remaining", "2 minutes remaining", "Auction extended to 18:02".
  - Bid history is a table with a caption. Images have alt text from the manifest; decorative icons are hidden.
- **Forms.** Visible labels, required marking, helper text, errors below fields tied by `aria-describedby`, `aria-invalid`, error summary for multi-field forms, correct `autocomplete` and `inputmode`, password paste and managers allowed, show/hide password.
- **Accessible bidding.** The whole flow works with a keyboard and a screen reader: field → quick increments (buttons that state the resulting amount, "Set bid to ₹43,00,000") → "Place bid ₹43,00,000" → inline confirmation receives focus → result announced and focus returns to the field. No drag, no hover-only content, no time limit on the confirmation other than the auction itself.
- **Touch.** 44px targets, 8px spacing, `touch-action: manipulation`.
- **Zoom and text size.** Works at 200% zoom and 320px width without horizontal scrolling; no fixed-height text containers; zoom is never disabled.
- **Motion.** Section 9.

---

## 11. Performance

Budgets (mid-range Android, 4G): LCP under 2.5 s on the lot page and Discover, CLS under 0.05, INP under 200 ms, initial JavaScript under 170 KB gzipped for the lot route.

- **Images.**
  - AVIF with WebP fallback, widths 400, 800, 1200, 1600, 2400, generated at build time for the interim manifest (and by the image service later).
  - `srcset` and `sizes` per usage: card `(min-width:1024px) 33vw, (min-width:768px) 50vw, 112px`; stage `100vw`.
  - Every image has an aspect-ratio box and explicit dimensions; no layout shift on load.
  - Placeholder: dominant colour from the manifest as background, optional 16px blur preview; fade to the image over 180 ms.
  - Lead photo on the lot page and the featured photo on Discover: `fetchpriority="high"`, not lazy, preloaded. Everything else `loading="lazy"` and `decoding="async"`.
- **Gallery.** Thumbnails lazy. The viewer loads the current image plus one on each side. The viewer component is code-split and loaded on first open.
- **Real-time.**
  - One socket per tab. Subscribe only to lots on screen in live contexts (lot page, featured, ending soon, account lists), not to every card in a grid.
  - Version check before applying. Field-level selectors so one bid updates three text nodes.
  - Burst protection: coalesce snapshots per animation frame; history refetch throttled to once per second.
  - Countdowns: one shared one-second ticker; each `Countdown` subscribes and updates its own text. Cards outside the viewport do not tick (IntersectionObserver). Cards with more than an hour left update once a minute.
  - Hidden tab: ticker stops, socket kept for 60 s then closed, state refreshed on return.
- **Rendering.** React Compiler on; manual memoisation only where profiling shows a need. Stable keys from ids. Lists stay below 100 rows per view; virtualise only if a list can exceed that (ledger "Load earlier" beyond 200 rows).
- **Loading.** Route-level code splitting (lot, browse, sell, account, auth). Prefetch a lot's data on card hover or focus (desktop) and when a card is within the viewport for 300 ms (mobile, on good connections only). Skeletons after 200 ms.
- **Pagination.** "Load more" on cursors, 24 per page for grids, 20 for bids, 50 for the ledger. No infinite scroll, so the footer stays reachable and position restores reliably.
- **Requests.** Debounced suggest with cancellation; search and list requests cancelled on param change; global limit awareness (20 requests per second per IP): the client never fans out more than 6 parallel lot fetches (matters for the account lists).
- **Fonts.** Two variable files, preloaded, subset, swap, metric-matched fallbacks.
- **Third parties.** None.

---

## 12. Implementation order (detail)

Each step ends in something runnable against `docker compose up` and the seeded API.

1. **Scaffold** `web/`: Vite, React, TypeScript, Tailwind 4, router, query client, generated API types, env (`VITE_API_URL`, `VITE_WS_URL`, `VITE_CURRENCY`), lint and type-check in CI.
2. **Tokens and primitives**: `tokens.css` (both themes, era attribute), fonts, `Button`, fields, `Sheet`, `Dialog`, `Toast`, `Skeleton`, `Table`, `EmptyState`.
3. **Domain layer** with unit tests: money, time and phase derivation, `minNextBid`, description parser, viewer derivation.
4. **API client and auth**: fetch wrapper, error model, token refresh (single flight, cross tab), login, register, check email, activate, session expiry.
5. **Lot page, read-only**: stage, placeholder, header, specs, description, bid history, static bid panel from REST.
6. **Realtime**: socket manager, live store, countdown ticker, connection states. The lot page is now live.
7. **Bidding**: `BidForm`, bid machine, idempotency, every response in 4.5, outbid and result states, mobile bid bar and sheet.
8. **Wallet**: balance in nav, wallet page, deposit, ledger, "Add funds" from the bid panel.
9. **Lot card and Browse**: grid and list layouts, status tabs, category, sort, load more, states.
10. **Search**: type-ahead, results page, highlights, empty and error states.
11. **Discover**.
12. **Account**: bids (ledger-derived), listings with cancel, settings.
13. **Watchlist** (local).
14. **Sell**: wizard, draft autosave, validation mapping, review, publish.
15. **Photos**: manifest, responsive pipeline, gallery and viewer.
16. **Hardening**: accessibility pass with keyboard and a screen reader, reduced motion, performance budgets, 375 / 768 / 1024 / 1440 checks, Playwright flows (register → activate → deposit → bid → outbid from a second session → win).

---

## 13. Open questions and backend dependencies

### 13.1 Decisions needed from you

1. **Reference images.** They did not arrive. Please attach them so section 2 can be checked against the real thing.
2. **Currency.** Amounts are integers in the "smallest currency unit" with no currency stated. This plan assumes INR in paise, formatted as `₹42,00,000`, set by `VITE_CURRENCY`. Confirm.
3. **Vehicle data.** Accept the interim convention (structured header inside `description`, `item.type` as `classic` / `modern`), or add backend fields first?
4. **Photos.** Accept the static manifest for seeded demo lots, or build upload first? This is the largest gap between the brief and the backend: a car auction with no photos cannot look premium.
5. **Product name and wordmark.** The prototypes use "Aurelian". Keep it?
6. **Default theme.** Paper (light) by default with a Carbon option, as planned, or follow the system setting from the first visit?
7. **Refresh token storage.** `localStorage` now, or move to an httpOnly cookie on the backend first?

### 13.2 Backend changes, in priority order

| # | Change | Unlocks | Until then |
|---|---|---|---|
| 1 | Item images: storage, upload endpoint, `images[]` on the auction response | Real photos, Sell photos step | Static manifest, placeholder |
| 2 | Structured item attributes (year, make, model, mileage, transmission, fuel, body, location), indexed in search | Real specs, real filters | Description header parsing |
| 3 | List and search params: `type`, `sort` (`ends_at`, `bid_count`, `current_bid`), ranges, `bidder=me` | Sort, filters, Account bids | Client sort over 100 lots, ledger-derived bids |
| 4 | Activation email links to the frontend (`{APP_BASE_URL}/activate?token=`) instead of the JSON endpoint; add the frontend origin to `CORS_ALLOWED_ORIGINS` and the WebSocket origin patterns | Activation flow | Users land on raw JSON |
| 5 | Server time (expose `Date` via CORS, or add `server_time` to auction responses and snapshots) | Skew-proof countdowns | Client clock |
| 6 | `min_next_bid` on the REST auction response | One source for the minimum | Client-side rule |
| 7 | Password reset endpoints | Forgot password | Not offered |
| 8 | Watchlist endpoints | Cross-device watchlist | Local storage |
| 9 | Public display names for bidders and sellers | Readable history, seller card | "Bidder a41c" |
| 10 | Reserve price and `reserve_met` in the snapshot | Reserve states | Not shown |
| 11 | Maximum (proxy) bids | "Your maximum" | Not shown |
| 12 | Total counts on list and search | "84 lots" | "Showing 24" |
| 13 | Edit a scheduled auction | Fix mistakes | Cancel and relist |

### 13.3 Things to verify during implementation

- That `GET /v1/auctions/{id}` responses are cached by Redis for only a short time, so a REST refetch after reconnect is not stale against the last snapshot (the version check protects against regressions either way).
- The ledger sign convention and which entries appear for the seller on settlement, before writing the plain-language ledger labels.
- Whether `auction.settled` produces a WebSocket message (it is an outbox event; confirm it is routed to the realtime handler).
- Behaviour of `suggest` when Elasticsearch is down.

---

## 14. Review against the two skills

### 14.1 `/ui-ux-pro-max`

Searches run: design system for "premium automotive auction marketplace collector cars" (density 7, motion 2, variance 4); product "auction marketplace bidding"; typography "luxury automotive editorial"; colour "automotive luxury marketplace"; UX queries on live updates, filters, images, feedback, sticky bars and multi-step forms; React stack guidance.

| Recommendation | Decision |
|---|---|
| Priority 1 accessibility: 4.5:1, keyboard, labels, focus, no colour-only meaning, live status phrased in context | Adopted, section 10 and ratios in 5.1 |
| Priority 2 touch: 44px targets, 8px spacing, loading feedback on buttons, no hover-only | Adopted, sections 5.7, 7 |
| Priority 3 performance: AVIF/WebP, lazy loading, reserved space, CLS under 0.1 | Adopted and tightened, section 11 |
| Priority 5 layout: mobile-first, 375 / 768 / 1024 / 1440, no horizontal scroll, 4px spacing scale, `dvh` | Adopted, sections 5.3, 7 |
| Priority 6: semantic colour tokens, tabular figures for prices and timers, 16px base | Adopted, 5.1, 5.2 |
| Priority 7: motion conveys meaning, at most 1 to 2 animated elements per view, transform and opacity only, exit faster than enter, reduced motion | Adopted, section 9 |
| Priority 8 forms: visible labels, errors by the field, error summary with focus, validate on blur, multi-step progress, draft autosave, confirm destructive actions | Adopted, 4.6, 4.9, 10 |
| Priority 9 navigation: deep links, state preservation, bottom nav of 5 or fewer with labels, one primary action per screen, focus on route change | Adopted, section 3 |
| React: React Compiler first, stable keys, measured memoisation | Adopted, section 11 |
| Auction product profile: dark ground, bid green, outbid red, countdown amber, "live feed plus countdown" landing | State colours adopted. "Live feed plus countdown" is Discover's ending-soon list. Dark-only is rejected: classic lots need the light ground. |
| Automotive palette: slate with action red | Red adopted as a state colour; slate replaced with warm neutrals from the brief |
| Design-system output: "Liquid Glass" style, "Scroll-triggered storytelling" pattern, Cormorant + Montserrat, gold accent, GSAP scroll reveal | **Rejected.** They contradict the brief (no glass, no storytelling hero, no decorative motion) and belong to a brand site, not an auction tool. Recorded here so the next session does not re-adopt them. |
| Pre-delivery checklist (no emoji icons, cursor on clickables, hover transitions 150 to 300 ms, focus visible, reduced motion, four breakpoints) | Adopted; hover kept at 120 ms because only colour changes |

The skill's `--persist` option (which writes `design-system/<project>/MASTER.md`) was not used: the brief allows only `plan.md`, and the generated master would contain the rejected style above. This document is the source of truth.

### 14.2 `/frontend-design`

| Guidance | How the plan follows it |
|---|---|
| Ground choices in the subject | Catalogue serif for collector lots, a width-axis grotesque for badge-like modern titles, tabular figures as instrument readouts, the extension meter derived from the engine's real anti-sniping rule |
| Open with the most characteristic thing | Discover opens on a live lot photograph with its bid and time, not a headline |
| Typography carries personality; two families at most, clearly distinct | Fraunces and Archivo only |
| Avoid the tells: all-caps eyebrows, monospace data labels, middle-dot meta strings, arrows on links, single accented word, numbered markers on non-sequences, identical rounded shadowed cards, gradient washes | None are used. Numbered steps appear only in Sell, which is a real sequence. Meta facts are separated by spacing. |
| Warm cream + serif + terracotta is a generated default | The brief explicitly asks for ivory, charcoal and muted red, so the brief wins. The plan departs from the default where it is free to: the red is a deep signal red reserved for state rather than a clay accent, the paper is the project's own existing neutral, primary actions are ink, and half the product runs on a dark stage. |
| Near-black + one bright accent is a generated default | Used only for the modern stage and the optional dark theme, as the brief requires |
| Spend boldness in one place | The bid panel. Everything around it is quiet. |
| Motion sparingly; one orchestrated moment at most | No entrance motion at all; motion only answers state changes |
| Copy: active voice, buttons say what happens, same name through the flow, errors give direction, sentence case | Section 5.10 and the exact strings in section 4 |
| Quality floor: responsive, keyboard focus, reduced motion, accessible contrast | Sections 7, 9, 10 |
| Remove one accessory | Removed from the brief's candidate list: separate VehicleCard, numbered pagination, "Popular" and "No reserve" rows, seller ratings, share counts, dashboard analytics, 3D or video showcase on lots |

---

## Implementation Priority

1. **Foundation**: scaffold, tokens, fonts, primitives, domain helpers (money, time, phase, minimum bid).
2. **API client and sign-in**: login, register, activation, token refresh.
3. **Lot page, read-only**: stage, title, specs, description, bid history, bid panel showing the four numbers.
4. **Live updates**: WebSocket, versioned snapshots, countdown, connection states.
5. **Placing bids**: bid form, confirmation, idempotency, accepted / rejected / outbid / ended states, mobile bid bar and sheet.
6. **Wallet**: balance, add funds, ledger, insufficient-funds path.
7. **Lot card and Browse**: grid, status, category, sort, load more.
8. **Search**: type-ahead and results.
9. **Discover**.
10. **Account**: my bids, my listings, settings.
11. **Watchlist** (device-local).
12. **Sell** flow.
13. **Photos**: manifest, responsive images, gallery viewer.
14. **Hardening**: accessibility, performance budgets, cross-device checks, end-to-end tests.
