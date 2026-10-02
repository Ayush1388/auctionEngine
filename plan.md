# Frontend plan: auctionEngine

A premium automotive auction house on top of the existing Go auction engine. This document is the design and implementation plan only. No application code, components or dependencies were created.

Rule that settles every disagreement below: **car = emotion, UI = clarity, auction = urgency and trust.** When "more impressive" and "easier to browse or bid" pull apart, the second wins.

---

## 0. Inputs, and what this plan is based on

| Input | Status | Notes |
|---|---|---|
| Repository | Read | `github.com/Ayush1388/auctionEngine`, `main` at v1.0 (commit `96c4e78`). The local checkout in `Desktop/auctionEngine` is older (pre-bidding README, no `api/`), so pull before implementing. |
| API contract | Read | `api/openapi.json`, plus handlers, bidding rules, realtime hub, config, migrations. |
| Existing frontend code | Read | None in the repo. Local-only prototypes exist: `aurelian-landing/`, `aurelian-duality/` (Vite 8, React 19, TypeScript, Tailwind 4, GSAP) and `watch/` (react-three-fiber). They are watch-themed showpieces, not an app. Their stack is reused; their `ink #14130F` is carried over. |
| `/frontend-design` skill | Read and applied | Subject-grounded choices, one bold element, restraint, copy rules, the list of generated-design tells to avoid. |
| `/ui-ux-pro-max` skill | Read and applied | Priority table, full quick reference (10 categories), and searches against its database. Results used and results rejected are listed in section 14. |
| Reference images | Received and studied | Six images, labelled A to F in section 2. They arrived after the first version of this plan; section 2.5 lists exactly what they changed. |

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

Six reference images were studied. They are references for principles, not templates: no layout, logo, wording, brand name or proprietary element from them is reproduced.

| Ref | What it shows | Direction |
|---|---|---|
| A | A browse page: category tiles with line drawings of cars, a sort select, and a three-column grid of classic-car auction cards on white | Classic |
| B | A landing screen on cream with a very large red condensed wordmark, small typewriter-style navigation, a countdown, and a wide photo band of a 1950s car with a three-thumbnail selector | Classic |
| C | A type and colour board: a tall condensed sans, and three swatches (ivory `#F8FAED`, red `#AD221D`, black `#000101`) on a frosted panel over a desert road photo | Classic |
| D | A dark auction-house event page on desktop and phone: crowd photo hero, icon-and-label section links, a "docket" toolbar (per-page, search, all filters, sort), and lot cards with studio photos | Modern |
| E | Bidder registration packages: light cards with icon checklists and black buttons over a red-tinted event photo, and a phone version | Modern (light surfaces) |
| F | A dark lot detail page for a modern supercar: a row of small tags, a bold uppercase title, in-page tabs, previous / next lot controls, one large studio photo, a details grid, and a photo mosaic | Modern |

### 2.1 Classic and vintage direction (A, B, C)

**What works, and why.**

| Observed | Why it works |
|---|---|
| A: every card has the same five-part order: photo, location, serif title, three label-and-value rows, then a tinted footer with bids and price on the left and time left on the right | The eye learns the pattern on the first card and then only reads values. Three cards can be compared row against row. |
| A: label on the left, value right-aligned, a hairline between rows | Right-aligned values form a column. It is a specification sheet, which is exactly how collectors read a car. |
| A: time left is the only red text on the card | Urgency is the one thing that is coloured, so it is seen first without any badge. |
| A: the serif appears only in titles and prices; everything else is a plain sans | The serif reads as "catalogue" because it is rare. |
| A: categories are tiles with a small side-profile line drawing and a label | A drawing of a body shape is recognised faster than a word, and it gives the page automotive character without decoration. |
| A: photos are taken on location (workshop wall, forest road, rusted steel door), wide crop, car filling the frame | Context gives a classic car its story. The UI adds nothing on top. |
| A: white cards on a white page, separated by a border and a soft shadow, footer in a cool grey tint | The tinted footer makes the money and the clock a distinct zone of the card. |
| B: the screen is two flat fields, cream above and photograph below, with no boxes | Large calm areas feel expensive; the photograph is the only complex thing on screen. |
| B: a thin strip of small facts (season, a tagline, a countdown, one action) sits between the type and the photo | A metadata strip is a compact, magazine-like way to carry status next to an image. |
| B: three small thumbnails at the photo's left edge select which car is shown | The viewer chooses; nothing rotates by itself. |
| C: three colours only, with red sitting between ivory and black | A palette this small makes any use of red deliberate. |

**Principles adopted, and where they land.**

| Principle | Translation in this product |
|---|---|
| A fixed card anatomy with spec rows | Lot card rewritten in 4.3: location line, title, up to three label-and-value rows, tinted footer band |
| Red is spent on time and trouble, nowhere else | Already the plan's rule (principle 3); reference A confirms it. The accent is now the reference red `#AD221D` (5.1) |
| Serif for titles and money labels only | Fraunces for lot titles (5.2); figures stay in the sans with tabular numerals so columns align, which a text serif does not do as well |
| Category tiles with line drawings | `CategoryTiles` replaces the plain text strip (4.1, 4.2, 6.2) |
| Lighter ivory ground, white cards, tinted footer | Paper theme lightened from a grey-beige to an ivory, cards on white, footer on `bg-secondary` (5.1, 5.6) |
| Flat fields and a metadata strip | The featured lot on Discover: photo, then a one-line strip of bid, bids, time (4.1). The classic lot stage: photo on a flat paper field (5.8) |
| A manual thumbnail selector | Discover's featured block can switch between the three most active lots (4.1) |
| A condensed display sans as the brand voice | The wordmark only: Archivo at narrow width, heavy weight (5.2). Not used for content. |
| Location photography | Photography direction for classic lots (5.8) |

**Explicitly not copied.**

- B's giant red wordmark filling half the first screen. It is a landing-page gesture that pushes lots down, and it spends red on branding. Here the largest text on Discover is a lot title.
- B's typewriter-style navigation in tiny capitals, its pill-outlined buttons and its two-line menu icon. Navigation here is ordinary, labelled and at readable size; buttons are 4px rectangles.
- A's tiny bold capitals for "12 BIDS", "TIME LEFT" and the location. The same information is kept in sentence case at 12px so it stays readable at small sizes and in translation.
- A's drop shadow under cards. Border and background step only.
- C's frosted glass panel over a photograph. No blur or translucency anywhere.
- Any sepia, grain or paper texture. The warmth comes from the photographs and the ivory ground.
- "Excessively beige" is avoided by keeping cards white and by the dark ink of text and buttons: ivory is a ground, not a tint on everything.

### 2.2 Modern and exotic direction (D, E, F)

**What works, and why.**

| Observed | Why it works |
|---|---|
| F: one studio photograph, car in three-quarter front view on a neutral grey sweep, nearly full width | With no background story, the paint, the surfacing and the stance are the whole image. The black page makes the red car the brightest thing on screen. |
| F: the title is short, bold, uppercase sans; nothing else on the page is that heavy | A model name reads like a badge on the car. |
| F: a row of small rectangular tags above the title carries status facts (reserve, lot, status, location) as "label: value" | Status is read before the title, in one line, without icons. |
| F: in-page tabs (details, photos, description…) under the title, and "back to results" with previous and next lot at the right | A long lot page becomes navigable, and browsing a catalogue lot by lot does not need the back button. |
| F: the single primary action is a white button on black, beside the title | The one filled light shape on a dark page cannot be missed. |
| F: details as a three-column grid, value on top in white, label underneath in grey, hairlines between rows | Values are what people scan for; labels are a caption. More compact than a two-column table. |
| F: a photo mosaic (large and small tiles: interior, engine, wheels, details) fills the lower page | Bidders inspect a car by its photographs. Seeing them all at once is faster than paging through a carousel. |
| D: lot cards on dark: studio photo on light grey, a thin meta row (reserve tag, lot number), then event and an uppercase title | Uniform studio backgrounds make a mixed grid tidy even on black. |
| D: a toolbar above the grid: items per page, search, one "All filters" button, sort | Filters cost no horizontal space until they are wanted, so the grid keeps full width. |
| D: on the phone, two equal outlined buttons side by side and a horizontally scrolling row of section links | Primary choices stay in thumb reach and at full size. |
| E: a 3 to 4px coloured top edge marks the featured card; a black filled button on a white card; icon checklists | A coloured edge is a quiet way to mark state without filling a surface. Black on white is the strongest button on a light surface. |

**Principles adopted, and where they land.**

| Principle | Translation in this product |
|---|---|
| A true black stage for modern lots | New `stage-dark` token, darker than the Carbon theme background (5.1, 5.8) |
| Uppercase, heavy, short title as a badge | Modern lot titles are set uppercase in Archivo at wide width (5.2, 5.8). Classic titles keep their written case in the serif. |
| A facts row above the title | `LotContextBar` on the lot page: lot reference, era, location as plain rectangular tags (4.4) |
| In-page section navigation, back to results, previous and next lot | `SectionNav` and `LotPager` on the lot page (4.4) |
| Value-over-label spec grid | `SpecGrid` replaces the two-column table on the lot page (4.4). Cards keep label-left, value-right rows from reference A, because a card is narrow. |
| A photo mosaic for inspection | New "Photos" section on the lot page with a mosaic, in addition to the stage (4.4) |
| Filters behind one button until they are needed | Phase 1 Browse has a toolbar and no filter rail; the rail arrives only when structured filters exist (4.2) |
| The primary action is the one light shape on dark, the one dark shape on light | Already the plan's `action` token (ink on paper, paper on carbon); confirmed by D, E and F |
| A coloured edge marks state | Already the plan's 2px edges for leading, outbid and final minutes; confirmed by E |
| Studio photography for modern lots | Photography direction (5.8) |

**Explicitly not copied.**

- D's event hero: a full-height crowd photograph with a centred headline, date, sponsor and two buttons. It is a giant hero that hides the auctions, and this product has no events.
- D and F's red glow gradients behind the header and under cards, and E's red-tinted photo overlay. No decorative gradients; the only fade is the functional one in 5.8.
- F's photo watermarks, the "financing" tab, and the "request bidder info" price tag. None exist in this product.
- D and F's "No Reserve" tag and sequential lot numbers. The backend has no reserve price and no lot numbers; the lot reference is a short id.
- E's tiered packages, "New" badge, and translucent cards over a photograph. There is no bidder registration tier; anyone with an activated account and funds can bid.
- Icons beside every spec and every menu item. Icons here are limited to controls and status.
- The reference's small grey text on black where contrast falls below 4.5:1. All secondary text here meets the ratios in 5.1.
- A brand script logo. The wordmark is plain type.

### 2.3 How the two directions coexist in one system

Looking at A and F side by side, the things that differ are the ground (ivory against black), the photograph (location against studio), and the title voice (serif against uppercase sans). Everything that carries auction information can be the same component in both. That is how the system is built:

| Layer | Same for every lot | Changes with the lot |
|---|---|---|
| Navigation, search, toolbar, category tiles | Yes | |
| Lot card (anatomy, spec rows, footer band, figures, status) | Yes | Only the photograph |
| Bid panel, countdown, status, extension meter, bid history | Yes | |
| Spec grid, description, section nav, pager | Yes | |
| Buttons, forms, spacing, grid, radius, type scale | Yes | |
| Lot stage background | | Paper field for classic, true black for modern |
| Lot stage crop | | 16:9 classic, 21:9 modern |
| Title face and case | | Serif as written for classic, wide sans uppercase for modern |
| Photography | | On location for classic, studio for modern |

- Grids mix eras freely. A card never changes with era, so a 1959 bus beside a 2023 supercar still reads as one catalogue (the mixed grid in reference D shows this working).
- The site theme (Paper or Carbon) is the viewer's choice and is independent of era. Era only sets the stage of a lot page (5.8).
- There is one red, one green, one amber, with the same meanings on both grounds.

### 2.4 Lessons for information density

Reference A puts nine facts on a card (location, title, three specs, bids, price, time, watch) and stays calm because of alignment and repetition, not whitespace. Reference F puts eight specs in three short rows. The plan follows both: facts are aligned into columns and rows separated by hairlines, labels are small and quiet, values are dark, and no fact is placed in a decorative container.

### 2.5 What the references changed in this plan

| Section | Change |
|---|---|
| 2 | Rewritten from the actual images |
| 3.2 | Wordmark treatment |
| 3.3 | `Engine` added to the description header keys |
| 4.1 | Featured block gains a three-lot selector; categories become tiles |
| 4.2 | Phase 1 uses a toolbar and a full-width grid instead of a filter rail |
| 4.3 | Lot card rebuilt around reference A's anatomy; photo ratio 3:2 |
| 4.4 | Context bar, section navigation, lot pager, value-over-label spec grid, photo mosaic |
| 5.1 | Lighter ivory Paper theme, reference red `#AD221D`, `stage-dark` token |
| 5.2 | Uppercase modern titles, wordmark face, card label sizes |
| 5.6, 5.8 | Card footer band; stage background, bottom fade and photography direction per era |
| 6.2, 7, 9, 10, 11 | New components and their responsive, motion, accessibility and image rules |

Unchanged because the references confirmed them or did not bear on them: red reserved for state, ink primary buttons, flat cards, the bid panel and all bidding states, live update handling, authentication, wallet, account, sell flow, backend constraints.

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
- Wordmark: plain type, Archivo at narrow width (70) and weight 800, uppercase, in `text` colour, 20px. The condensed heavy sans is the one thing taken from the display type in references B and C; it is not used at poster size and not in red.
- No mega menu, no icons-only items, no notification bell (there is no notification API). No dropdown per top-level item (reference D has six; this product has two destinations).
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
- `item.type`: `classic` or `modern`. This drives the category tiles and the atmosphere. It is searchable (weighted B in PostgreSQL, ×2 in Elasticsearch).
- `item.description`: optional header of `Key: value` lines, a blank line, then prose.

```
Make: Porsche
Model: 911 S
Year: 1970
Mileage: 84,200 km
Engine: 2.2 L flat-six
Transmission: 5-speed manual
Fuel: Petrol
Body: Coupé
Location: Pune, Maharashtra

Matching-numbers example finished in Tangerine…
```

The parser is tolerant: unknown keys are shown as given, missing keys are omitted, a description with no header is all prose. Cards show the first three available of Mileage, Engine, Transmission, Fuel. The Sell flow writes this header from form fields so sellers never type it by hand.

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
│ [▫]▓▓▓▓▓▓▓▓  featured lot  ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ │ ▓▓ 1984 Nissan Sunny   04:12  │
│ [▫]▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ │    ₹3,10,000   14 bids        │
│ 1970 Porsche 911 S 2.2 Coupé                  │ — — — — — — — — — — — — — — — │
│ Current bid ₹42,00,000   31 bids   2d 4h left │ ▓▓ 2019 McLaren 720S   18:40  │
│                                               │ … 5 rows                      │
├───────────────────────────────────────────────┴───────────────────────────────┤
│ [▭ All lots] [▭ Classic] [▭ Modern]   (category tiles, link to /auctions?type=)│
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

  The featured block is two flat fields, as in reference B: the photograph, edge to edge inside its columns with no frame, and beneath it the title and a single strip of three figures (current bid, bids, time left) on the page background. Three 56px thumbnails (marked `[▫]`) sit on the photo's left edge and select which of the three most active lots is featured. Selection is manual only; nothing rotates on a timer. The featured block takes the atmosphere of the selected lot in title face only (serif or wide uppercase sans); it does not change the page background.

  Category tiles (from reference A): a rectangle with a small side-profile line drawing on a `bg-secondary` square at the left and the label at the right, 56px high, 2px radius, 1px border. Drawings are original, single-weight line art in `text` colour: a mixed pair for "All lots", a 1960s coupé for "Classic", a low mid-engined car for "Modern". When more categories exist they join the same row, with "Show more" after the fourth.

- **Information hierarchy.** Featured photo, featured title and its three figures, ending-soon list, categories, card rows.
- **Data sources.**

| Section | Source | Note |
|---|---|---|
| Featured | First three items of `GET /v1/auctions/trending`; fallback: newest `ACTIVE` lots. With fewer than three, the selector shows only what exists; with one, no selector. | |
| Ending soon | `GET /v1/auctions?status=ACTIVE&limit=100`, sorted by `ends_at` on the client | Interim, see 13 |
| Most active now | `GET /v1/auctions/trending?limit=8` | |
| Newly listed | `GET /v1/auctions?status=ACTIVE&limit=8` (already newest first) | |
| Starting soon | `GET /v1/auctions?status=NOT_ACTIVE&limit=8` | |
| Recently sold | `GET /v1/auctions?status=COMPLETED&limit=8` | |

  "No reserve" and "Popular" rows from the brief are dropped: reserve does not exist, and "popular" is the trending row.
- **Components.** `LotStage` (compact variant), `FeaturedSelector`, `EndingSoonList`, `CategoryTiles`, `LotRow`, `LotCard`.
- **Interactions.** Everything is a link to a lot or to Browse, except the featured selector, which swaps the featured lot in place. The featured lot and ending-soon list are subscribed to the WebSocket (at most 6 subscriptions on this page); card rows are not live, they refresh on focus and every 30 seconds.
- **States.** Loading: skeletons with the exact geometry of each block. Empty section: the section is omitted, except when there are no live auctions at all, where the page shows "No auctions are live right now" with the "Starting soon" row promoted to the top and a link to Sell. Error: inline "Could not load auctions. Try again" with a retry button in place of the section.
- **Desktop.** As drawn.
- **Tablet.** Featured full width at 16:9, ending-soon becomes a horizontal row of 3 compact items beneath it, card rows 3 across.
- **Mobile.** Featured lot at 3:2 full bleed, figures beneath. Ending soon as a vertical list of 3 with "See all". The featured selector moves below the photo as three thumbnails in a row. Category tiles become a three-across row of compact tiles (drawing above label). Card rows become single-column lists of 4 with "See all" links, not carousels (`gesture-conflicts`).

### 4.2 Browse (`/auctions`) and Search results (`/search`)

One page component, two entry points. Search adds a query; everything else is identical.

- **Purpose.** Scan many lots quickly and narrow them.
- **User goal.** Find lots worth opening.
- **Layout (desktop).**

```
┌ nav ──────────────────────────────────────────────────────────────────────────┐
│ Auctions                                                                       │
│ [▭ All lots] [▭ Classic] [▭ Modern]                                            │
│ ——————————————————————————————————————————————————————————————————————————————│
│ Live   Starting soon   Sold          Showing 24      [ Filters ]  Sort: Newest ▾│
│ ——————————————————————————————————————————————————————————————————————————————│
│ [card] [card] [card]                                                           │
│ [card] [card] [card]                                                           │
│ [card] [card] [card]                                                           │
│                              Load more                                         │
└───────────────────────────────────────────────────────────────────────────────┘
```

  This follows references A and D: categories as tiles, then one toolbar row, then a full-width grid. There is no filter rail in phase 1, because only two filters are real (status and category) and both are already in the header. The grid gets the width instead, which the richer card (4.3) needs.

- **What is real today versus planned.**

| Control | Phase 1 (works now) | Needs backend |
|---|---|---|
| Search box with type-ahead | `suggest` after 2 characters, debounced 200 ms; Enter goes to `/search?q=` | |
| Status | `status` param: Live, Starting soon, Sold. Cancelled only under "My listings". | |
| Category tiles (All lots, Classic, Modern) | Search with the category word added to `q` when a query exists; otherwise client filter on `item.type` over loaded pages | `type` param on list and search |
| Sort: Newest | Native order | |
| Sort: Ending soon, Most bids, Price | Client sort over the first 100 active lots, labelled "of the first 100" when more exist | `sort` param |
| "Filters" button: Make, Model, Year, Price range, Body, Location, Transmission, Fuel | **Not shown in phase 1.** The button is absent until at least one of these filters works. | Structured fields + filter params |
| Reserve / no reserve | Not shown | Reserve price |

  When structured filters exist, "Filters" opens a panel: on desktop a 264px rail that pushes the grid (grid drops from 4 to 3 columns at 1440, 3 to 2 at 1024) and stays open while browsing; on tablet a side sheet; on mobile a bottom sheet. Each group is a collapsible fieldset with a count of selected values; active filters appear as removable chips under the toolbar with "Clear all". The button shows the number of active filters: "Filters (2)".
- **Pagination.** Cursor based, so "Load more" (button, 24 per page), not numbered pages and not infinite scroll. The button keeps focus position and announces "24 more lots loaded". Scroll position and loaded pages are restored on back.
- **Search results specifics.** Heading: `Results for "porsche 911"` with the count unknown (the API returns no total), so show "Showing 20" and "Load more" while `next_cursor` exists. Each card shows the API's `highlight` snippet under the title, with `<em>` rendered as a background tint, not italics, after sanitising to allow only `em`. If `backend` is `postgres`, nothing is shown to the user; it is only logged.
- **Components.** `SearchBar`, `CategoryTiles`, `BrowseToolbar` (`StatusTabs`, `SortSelect`, filters button), `LotGrid`, `LotCard`, `LoadMore`, `EmptyState`. Later: `FilterPanel`, `ActiveFilterChips`.
- **States.**
  - Loading: 9 card skeletons; tiles and toolbar render immediately.
  - Empty (no lots for filters): "No lots match these filters" with "Clear filters".
  - Empty (search): `No lots match "…"`, a hint ("Try the make or the year, for example 911 or 1970") and a link to all live auctions.
  - Error: "Search is unavailable right now. Try again" with retry. 400 for a query under the minimum length is prevented in the UI.
  - Rate limited (429): "Too many requests. Trying again in N seconds" using `Retry-After`, then automatic retry once.
- **Desktop.** Toolbar sticks under the nav while scrolling. Grid 3 columns from 1024, 4 columns from 1440.
- **Tablet.** Same toolbar; grid 2 columns at 768.
- **Mobile.** Category tiles in one row of three. Sticky sub-bar under the top bar: status as a segmented control, and "Sort" as a native select (plus "Filters (2)" beside it once filters exist, the two as equal-width buttons as on the phone in reference D). Filters, when they exist, open as a full-height bottom sheet with a sticky "Show 31 lots" button and "Clear"; closing by swipe or the close button. The grid is a single column of full cards (4.3), so the photograph stays large; the compact list layout is for the ending-soon and account lists.

### 4.3 Lot card

One component, two layouts. The anatomy follows reference A: a fixed five-part order so cards compare row against row. No shadow, no radius above 2px, no hover lift.

Grid layout (desktop, tablet, mobile browse):

```
┌──────────────────────────────┐
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│  1  photo, 3:2, object-fit cover
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│     top-left: status tag, only when not plain "live"
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│
├──────────────────────────────┤
│ Pune, Maharashtra          ♡ │  2  location (12px, muted) and watch button
│ 1970 Porsche 911 S 2.2 Coupé │  3  title, serif 18px, 2 lines max
│                              │
│ Mileage            84,200 km │  4  up to three spec rows: label left (muted),
│ ———————————————————————————— │     value right (text), hairline between
│ Engine        2.2 L flat-six │
│ ———————————————————————————— │
│ Transmission          Manual │
├──────────────────────────────┤
│ 31 bids            Time left │  5  footer band on bg-secondary:
│ ₹42,00,000             2d 4h │     labels 12px, figures 20px semibold tabular
└──────────────────────────────┘
```

List layout (ending-soon list, account lists, watchlist): photo 112×75 on the left, title and location, then bid and time on one row. No spec rows.

**Hierarchy, in reading order:** photo, title, current bid, time left, bid count, specs, location. The footer band is the card's instrument: it is the only tinted area, so the money and the clock are found in the same place on every card.

**Footer band rules.**

- Left: the bid count is the label ("31 bids", or "No bids yet"), the amount is the figure. Before the first bid the figure is the starting bid and the label reads "Starting bid".
- Right: "Time left" label, time figure. The time figure is `text` colour normally and red in endingSoon and finalMinutes, as in reference A where the clock is the only red on the card. Because this product also shows lots with days left, red is not used for every clock, only for the last hour.
- Figures are right-aligned on the right side and left-aligned on the left, and never wrap. At narrow widths the label truncates before the figure does.

**Spec rows.** The first three available of Mileage, Engine, Transmission, Fuel from the description header. If a lot has no header, the three rows are omitted and the card is shorter; within one grid row, cards align their footer bands to the bottom so a short card does not break the line. No icons in spec rows.

**Activity marker.** Lots that appear in the trending list show a small rising-line icon beside the watch button with the accessible name "Bidding is active", taken from the trend marker on one card in reference A. It is `text-muted`, never red.

**Content rules by phase.**

| Phase | Footer left (label / figure) | Footer right (label / figure) | Tag on photo |
|---|---|---|---|
| scheduled | Starting bid / ₹… | Starts / 4 Oct, 18:00 | Starting soon |
| live | 31 bids / ₹… | Time left / 2d 4h | none |
| endingSoon | same | Time left / 42m 10s in red, with a red dot | Ending soon |
| finalMinutes | same | Time left / 01:47 in red, ticking | Final minutes |
| closing | same | Time left / Closing | Closing |
| sold | 31 bids / Sold for ₹… | Ended / 3 Oct | Sold |
| unsold | No bids / starting bid, muted | Ended / 3 Oct | Ended |
| cancelled | Starting bid / ₹… | Cancelled | Cancelled |

Tags on the photo are 2px-radius rectangles, `surface-elevated` background, `text` colour, 12px, with the red dot only for the two urgent phases. They are never pills and never filled red.

Viewer overlays (signed in): a 2px top edge on the footer band (green or red) and the footer's left label replaced by "You are leading" or "You have been outbid" with an icon. Colour is never the only signal.

Reserve status is not on the card (it does not exist). Location appears only when the description header provides it; otherwise the watch button sits alone on that line.

The whole card is one link (the title is the link, stretched over the card); the watch button is a separate control above it.

### 4.4 Lot page (`/auctions/:id`)

The most important screen. It is both the detail page and the live bidding experience.

- **Purpose.** Let someone judge the car and bid with confidence.
- **User goal.** Understand the lot, know exactly where the auction stands, place a bid, know whether it worked.
- **Layout (desktop, 1024 and up).**

```
┌ nav ──────────────────────────────────────────────────────────────────────────┐
│ ‹ Back to results                                        ‹ Previous   Next ›   │
├───────────────────────────────────────────────────────────────────────────────┤
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  stage: lead photo, full bleed  ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  1 / 24  ▓▓▓▓│
│ [thumb][thumb][thumb][thumb][thumb][thumb][thumb]  View all 24 photos          │
├──────────────────────────────────────────────────┬────────────────────────────┤
│ [Lot 7f3c2a1b] [Classic] [Pune, Maharashtra]     │ ● Live                      │
│ 1970 Porsche 911 S 2.2 Coupé            ♡ Watch  │ Time remaining              │
│                                          Share   │ 2d 04h 12m 08s              │
│ Specifications  Description  Photos  Bid history │ Ends Sun 4 Oct, 18:00 IST   │
│ ———————————————————————————————————————————————— │ ——————————————————————————  │
│ Specifications                                   │ Current bid                 │
│ Porsche      911 S        1970        84,200 km  │ ₹42,00,000                  │
│ Make         Model        Year        Mileage    │ 31 bids                     │
│ ———————————————————————————————————————————————— │ ——————————————————————————  │
│ 2.2 L flat-6 5-sp manual  Petrol      Coupé      │ Your bid                    │
│ Engine       Transmission Fuel        Body       │ [ ₹ 42,50,000            ]  │
│ ———————————————————————————————————————————————— │ Minimum next bid ₹42,50,000 │
│ Description                                      │ [+₹50,000] [+₹1,00,000] [+₹2,50,000]│
│ Matching-numbers example finished in…            │ [      Place bid ₹42,50,000     ]│
│ ———————————————————————————————————————————————— │ Available ₹60,00,000  Add funds │
│ Photos                                    24     │ ——————————————————————————  │
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓ ▓▓▓▓▓▓                     │ Extensions used 0 of 10     │
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓ ▓▓▓▓▓▓   (mosaic)          │ Bids in the last 2 minutes  │
│ ▓▓▓▓▓▓ ▓▓▓▓▓▓ ▓▓▓▓▓▓ ▓▓▓▓▓▓    Show all 24       │ extend the auction.         │
│ ———————————————————————————————————————————————— │                             │
│ Bid history                              31 bids │                             │
│ ₹42,00,000   Bidder a41c        2 min ago        │                             │
│ ₹41,50,000   You                5 min ago        │                             │
│ Show earlier bids                                │                             │
│ ———————————————————————————————————————————————— │                             │
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
  4. Context tags (lot reference, era, location).
  5. Specifications, description, photos, bid history, seller, related lots.

- **Top row: back and pager.** "Back to results" returns to the list the viewer came from, restored (scroll position, loaded pages, filters). "Previous" and "Next" step through that same list, so a catalogue can be walked lot by lot (reference F). When the lot was opened directly from a link, the row shows "All auctions" instead and no pager. Keyboard: the pager is ordinary links; no single-key shortcuts.

- **Context tags.** A row of up to three plain rectangular tags above the title, in the manner of reference F's tag row: lot reference, era, location. 12px, 2px radius, 1px border, no fill, no icons. Auction status is not repeated here; it lives at the top of the bid panel, a few pixels to the right, where the numbers it qualifies are.

- **Title block.** Title uses the era treatment (5.8): serif in written case for classic lots, wide uppercase sans for modern lots. Year, make and model are in the title; they are not repeated as separate fields above it.

- **Section navigation.** A row of text links under the title: Specifications, Description, Photos, Bid history. They are in-page anchors, not tabs: all sections stay on the page and in the document order, the links scroll to them. The row sticks under the nav on desktop once the title scrolls away, with the current section marked by a 2px underline in `text` colour. Sections without content (no photos) are omitted from the row.

- **Specifications.** A grid of value-over-label cells from the description header (reference F): value in `text` at 16px medium, label beneath in `text-muted` at 12px, a hairline between rows, no vertical rules, no icons. Four columns on desktop, three on tablet, two on mobile. Built as a description list so each value is read with its label. Order: Make, Model, Year, Mileage, Engine, Transmission, Fuel, Body, Location, then any other keys the seller supplied. Lots with no header show no Specifications section.

- **Photo gallery.** Stage shows the lead photo. Thumbnail strip beneath (7 visible at desktop). Lower on the page, the Photos section shows a mosaic (reference F): the first tile spans two columns and two rows, the rest are single tiles, 4 columns on desktop, 3 on tablet, 2 on mobile, 4px gaps, every tile a fixed 3:2 box. It shows up to 11 tiles, then "Show all 24". When the manifest groups photos (exterior, interior, engine, details), the mosaic is ordered by group with a small text label at the start of each group. Tiles are buttons that open the viewer at that photo. No watermarks. Click on stage, a thumbnail, a tile or "View all" opens a full-screen viewer: one image at a time, arrow keys and swipe, counter, close on Escape, focus trapped and returned. No autoplay, no zoom-on-hover lens, no 360 spin. With no photos, the stage becomes the typographic placeholder at reduced height (32vh) and the thumbnail strip is omitted.

- **Bid panel.** Described in 4.5.

- **Bid history.** `GET /v1/auctions/{id}/bids`, 20 newest, "Show earlier bids" loads the next page. Columns: amount, bidder, time. Bidder is "You" for the signed-in user, otherwise "Bidder" plus the first 4 characters of `user_id`, stable per auction page. Extended bids cannot be identified from the bid list, so no marker is shown. New bids arriving live are inserted at the top (9.1). The list is a table with a caption.

- **Seller.** Only `owner_id` and `created_at` exist: "Seller 9c1e" and "Listed 28 Sep". If the viewer is the seller: "This is your listing" with "Cancel auction" while it is scheduled. No ratings, no avatar, no contact button.

- **Watch.** Toggles the local watchlist (4.8). Label changes between "Watch" and "Watching".

- **Share.** Uses the Web Share API when available, otherwise copies the URL and shows "Link copied".

- **Documents / history.** Not supported; the section is absent, not shown empty.

- **Related lots.** Up to 3 active lots of the same `item.type`, from the list endpoint filtered on the client. Omitted when there are none.

- **Not found / cancelled.** 404: "This lot does not exist" with a link to live auctions. Cancelled: full page still renders, bid panel replaced by "This auction was cancelled before it started."

- **Tablet (768 to 1023).** Stage 16:9. Single column. Section navigation scrolls horizontally if it does not fit. Bid panel becomes a full-width block directly under the title, and a compact sticky bar appears at the bottom once that block scrolls out of view.

- **Mobile (below 768).**

```
┌──────────────────────────┐
│ ‹ Auctions        ♡  ⤴  │
│▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓│ swipeable gallery, 3:2, dots + "1 / 24"
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

  On mobile the back link and pager collapse into the top bar ("‹ Auctions"); previous and next are offered at the foot of the page as two equal outlined buttons. Context tags sit under the title on one line. The section navigation is replaced by the collapsible sections shown above, plus a Photos section with a two-column mosaic.

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
| `bg` | `#F4F3EC` | `#101113` | Page background |
| `bg-secondary` | `#E8E6DE` | `#17191C` | Card footer band, table header rows, category tile drawing square, skeleton base |
| `surface` | `#FFFFFF` | `#1C1F23` | Cards, bid panel, inputs |
| `surface-elevated` | `#FFFFFF` | `#262A2F` | Sheets, menus, dialogs, toasts |
| `text` | `#14130F` (16.7:1 on bg) | `#F2F0EB` (16.6:1) | Titles, figures, body |
| `text-secondary` | `#45433D` (8.9:1) | `#B8B5AD` (9.2:1) | Fact lines, descriptions, labels |
| `text-muted` | `#66635B` (5.4:1 on bg, 4.8:1 on bg-secondary) | `#9D9A92` (6.7:1; 5.1:1 on elevated) | Timestamps, helper text, bid counts |
| `border` | `#D3D0C6` | `#30343A` | Hairlines, dividers, card outlines (decorative, not relied on alone) |
| `border-strong` | `#7C786D` (4.0:1 on bg, 4.4:1 on surface) | `#737882` (4.3:1) | Input and control outlines (meets 3:1 for UI components) |
| `action` | `#14130F` with text `#F4F3EC` | `#F2F0EB` with text `#101113` | Primary buttons. Ink on paper, paper on carbon. |
| `accent` / `live` | `#AD221D` (6.2:1 on bg, 5.6:1 on bg-secondary; white on it 6.9:1) | `#F4665D` (6.2:1 on bg; 4.75:1 on elevated) | Live dot, ending soon, final minutes, outbid |
| `danger` | same as accent | same as accent | Errors and destructive actions, always with an icon and text |
| `success` | `#1D6A43` (5.9:1) | `#4FC48C` (8.6:1) | Bid accepted, leading, won |
| `warning` | `#855400` (5.8:1) | `#E3A63A` (8.8:1) | Reconnecting, extended, pending |
| `focus` | `#14130F` | `#F2F0EB` | 2px ring with 2px offset |
| `stage-dark` | `#070708` | `#070708` | Background of the modern lot stage only, in both themes. Text on it uses the Carbon text values (17.7:1, muted 7.2:1, red 6.6:1). |

The Paper values were refined after the references: the ground is a lighter ivory than the first draft (closer to reference C's `#F8FAED`, kept slightly greyer so white cards still separate from it), cards are white as in reference A, and the red is reference C's `#AD221D`. Reference C's near-black is used for the modern stage; the Carbon theme stays a step lighter than pure black so that surfaces, borders and elevation remain distinguishable.

Rules:

- **Red is state.** It appears only for live, ending soon, final minutes, outbid, errors and destructive confirmation. Never on primary buttons, links, logos, hover states or decoration. On any screen, visible red should mean something is live or needs attention.
- **Primary action is ink.** One primary button per screen.
- **Auction-live** = red dot + "Live". **Ending soon** = red time figure + "Ending soon" tag. **Final minutes** = the same plus a 2px red top edge on the bid panel.
- Green and amber appear only as text with an icon or as a 2px edge, never as filled backgrounds larger than a tag.
- No gradients, except one functional scrim: a bottom-up `text`-to-transparent overlay behind text that sits on a photograph.
- Why not the palettes the skill database suggested: its "Auction Platform" row proposes dark mode with bid green, outbid red and countdown amber, and its "Automotive" row proposes slate plus action red. The state colours are adopted. The slate and the gold-accent luxury palette are rejected in favour of the ivory, red and black of the references and the project's existing `ink`.

### 5.2 Typography

Two families, clearly different, both variable and available from Google Fonts under open licences.

| Role | Family and settings | Sizes (desktop / mobile) |
|---|---|---|
| Display (lot title on the lot page, featured lot) | **Fraunces**, weight 400, optical size auto, `SOFT` 0, `WONK` 0, tracking −0.01em | 44 / 30, line height 1.1 |
| Heading (section headings, card titles, page titles) | Fraunces 500 | 28, 22, 18 / 24, 20, 17, line height 1.2 |
| Display, modern lots | **Archivo**, weight 700, width 112, uppercase, tracking −0.01em | 40 / 26, line height 1.05 (uppercase runs wider, so one step smaller than the serif) |
| Wordmark | Archivo, weight 800, width 70, uppercase | 20 |
| Body | Archivo 400, width 100 | 16, line height 1.55; description measure 60 to 72 characters |
| Metadata (spec labels and values, card footer labels, location, table headers, tags) | Archivo 500, width 88 | 13 and 12, line height 1.35, sentence case |
| Numeric (bids, countdowns, balances) | Archivo 600, width 100, `font-variant-numeric: tabular-nums lining-nums` | 40 (panel current bid), 28 (countdown), 20 (card figures), 16 (table) |

- Why these: Fraunces gives the collector-catalogue warmth for classic lots without the fashion-brand feel of the high-contrast serifs the skill database suggested (Playfair, Cormorant; Cormorant is also what the watch prototype used). Archivo has a width axis, which lets one family be condensed for dense metadata and widened for the badge-like titles of modern cars, so "two atmospheres" costs no extra font.
- No monospace anywhere. Tabular figures do the job without the terminal look.
- No all-caps labels, no letter-spaced eyebrows. Labels are sentence case at metadata size. Uppercase appears in exactly two places: the wordmark and the title of a modern lot, where it works like a badge on the car (reference F). Uppercase titles are produced with CSS from the stored title, so screen readers and search still get the written case.
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
- Cards have a 1px `border` outline in Paper and none in Carbon (the surface step is enough). Inside a card, a hairline separates the photo from the text, each spec row from the next, and the text from the footer band; the footer band is the only tinted area (`bg-secondary`).
- A 2px coloured left or top edge is the only "accent border" and is reserved for viewer state (leading, outbid) and final minutes.

### 5.7 Icons and controls

- One icon set, 1.5px stroke (Lucide or Phosphor regular). Icons accompany text; icon-only buttons (watch on a card, close, gallery arrows) have accessible names.
- No emoji. No arrow glyphs appended to link text.
- Buttons: primary (ink fill on Paper, paper fill on Carbon and on the dark stage: the one light shape on a dark page, as in references D and F), secondary (1px `border-strong` outline), quiet (text only, underlined on hover), destructive (red outline; red fill only inside a confirmation dialog). Heights 44 default, 36 compact on desktop tables only.
- Inputs: 44 high, visible label above, helper or error text below, 1px `border-strong`, 2px `focus` ring.

### 5.8 Atmospheres: one system, two worlds

`data-era="classic" | "modern"` is set on the lot stage (and only there). It changes exactly these things:

| Property | Classic | Modern |
|---|---|---|
| Stage background (behind and around the photo, letterbox areas) | Paper `bg-secondary` (regardless of site theme) | `stage-dark` (regardless of site theme) |
| Stage aspect ratio (desktop) | 16:9 | 21:9 |
| Title face | Fraunces 400 | Archivo 700, width 112 |
| Title case and tracking | As written, −0.01em | Uppercase, −0.01em |
| Photo treatment | None. No filter, no vignette, no sepia. Hard edge against the paper field. | None on the image itself. The bottom 48px of the stage fades from the photo into `stage-dark` so a studio backdrop does not end in a hard line against black (the one fade in the product, from reference F). |
| Preferred lead photograph | On location: a wall, a road, a workshop. Three-quarter or side view. | Studio or plain backdrop, even light, three-quarter front view. |
| Gallery thumbnails | 2px radius, 8px gap | 0 radius, 4px gap |
| Rule under the title block | 1px `border` | 1px `border-strong` |

Everything else, including the bid panel, cards, specs, history, colours of state, spacing and controls, is identical. Cards in grids ignore era entirely. The site theme (Paper or Carbon) is the user's choice or system setting and is independent of era; a classic lot in Carbon theme keeps its paper stage, a modern lot in Paper theme keeps its dark stage. That contrast is the intended effect: the stage is a lit plinth, the page around it is the catalogue.

Photography direction (for seed content and seller guidance): car fills at least 70% of frame width, horizon level, no text or watermarks baked in, consistent 3:2 source crop with a 21:9-safe centre band for modern lots. The era preference above is guidance, not a rule: a classic car shot in a studio still gets the paper stage, and nothing in the UI depends on the kind of photograph. A set should cover exterior (front and rear three-quarter, both sides), interior, engine and details, in that order, which is also the mosaic order.

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
| `CategoryTiles` | All lots, Classic, Modern: line drawing plus label | Links, current item marked with a 2px `text` bottom edge |
| `FeaturedSelector` | Three thumbnails that choose the featured lot on Discover | A tab list; manual only |
| `BrowseToolbar`, `StatusTabs`, `SortSelect` | Browse controls bound to the URL | |
| `FilterPanel`, `ActiveFilterChips` | Later, when structured filters exist | One set of filter definitions rendered as rail, side sheet or bottom sheet |
| `LotGrid`, `LotRow` | Grid and list containers, skeleton counts | |
| `LotCard` | Section 4.3; `layout="grid" | "list"` | One component; replaces the brief's separate AuctionCard and VehicleCard, which would show the same data |
| `LotPhoto` | Responsive image, aspect box, blur placeholder, typographic fallback | Used by card, stage, thumbnails |
| `LoadMore` | Cursor pagination control with announcement | Replaces Pagination: the API is cursor based |
| `LotStage` | Lead photo with era atmosphere | Sets `data-era` |
| `PhotoStrip`, `PhotoMosaic`, `PhotoViewer` | Thumbnails under the stage, mosaic section, full-screen viewer | Mosaic tiles are lazy |
| `LotPager` | Back to results, previous, next | Reads the originating list from router state |
| `LotContextBar` | Lot reference, era, location tags | |
| `LotHeader` | Title, watch, share | |
| `SectionNav` | In-page anchor links, sticky, current section marked | |
| `SpecGrid` | Value-over-label cells from parsed specs | Description list; 4, 3, 2 columns |
| `SpecRows` | Label-left, value-right rows on the card | Up to three |
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
| Lot cards | Full card, single column (list layout only in ending-soon and account lists) | Grid, 2 columns | Grid, 3 columns; 4 at 1440 |
| Filters (when they exist) | Full-height bottom sheet, sticky apply button | Side sheet | 264px rail opened from the toolbar |
| Category tiles | Three across, drawing above label | One row | One row |
| Section navigation (lot) | Replaced by collapsible sections | Horizontal scroll row | Sticky row |
| Back and pager (lot) | Back in top bar; previous and next at page foot | Top row | Top row |
| Sort | Native select in the sticky sub-bar | Menu | Menu |
| Lot stage / gallery | Swipeable 3:2, counter, tap for viewer | 16:9 stage + thumbnail strip | 16:9 or 21:9 stage + strip |
| Bid panel | Summary block inline + sticky bid bar + bid sheet | Inline block under title + sticky bar after scroll | Sticky right column |
| Countdown | In the sticky bar and inline; compact format ("2d 4h") in the bar until under 1 hour | Full format | Full format |
| Bid history | 5 rows, "Show all" expands in place; time as relative only | 10 rows | 20 rows, absolute time on hover and focus |
| Specs | Two columns | Three columns | Four columns |
| Photo mosaic | Two columns | Three columns | Four columns, first tile 2×2 |
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
| Featured lot switched on Discover | 180 ms cross-fade of photo; title and figures swap instantly |
| Sheet, dialog, menu | Slide or fade in 180 to 240 ms, out faster |
| Toast | Fade and 8px rise in, fade out |
| Hover | Card: title underline. Button: background step. 120 ms. No scaling, no lift, no image zoom. |
| Route change | None. New content renders; focus moves to the page heading. |
| Skeleton | Slow opacity pulse (1.6 s); static under reduced motion |

### 9.2 Not used

Scroll-triggered reveals, parallax, staggered grid entrances, page transitions, number-rolling tickers, pulsing "live" dots, auto-advancing carousels, cursor effects, 3D, glows or animated gradients behind headers and cards (seen in references D and F). The design-system search returned a scroll-reveal GSAP preset and a "scroll-triggered storytelling" page pattern; both are rejected for this product.

Reduced motion: all transitions become instant, the price tint becomes a 1 s static outline, skeletons stop pulsing. No state depends on an animation finishing.

---

## 10. Accessibility

Target: WCAG 2.2 AA.

- **Keyboard.** Everything operable by keyboard in visual order. Skip link to main content. Type-ahead is a combobox with listbox semantics and arrow keys. Gallery viewer: arrows, Home, End, Escape. The featured selector on Discover is a tab list (arrow keys move, selection follows focus). Section navigation is a labelled `nav` of links with `aria-current` on the active one. Mosaic tiles are buttons named by their photo's alt text. Filters: fieldsets with legends. No keyboard traps except intentional focus traps in dialogs and sheets.
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
  - `srcset` and `sizes` per usage: card `(min-width:1440px) 25vw, (min-width:1024px) 33vw, (min-width:768px) 50vw, 100vw`; list thumbnail `112px`; mosaic tile `(min-width:1024px) 15vw, 50vw`; stage `100vw`.
  - Every image has an aspect-ratio box and explicit dimensions; no layout shift on load.
  - Placeholder: dominant colour from the manifest as background, optional 16px blur preview; fade to the image over 180 ms.
  - Lead photo on the lot page and the featured photo on Discover: `fetchpriority="high"`, not lazy, preloaded. Everything else `loading="lazy"` and `decoding="async"`.
- **Gallery.** Thumbnails and mosaic tiles lazy, each in a fixed 3:2 box; the mosaic renders at most 11 tiles until "Show all" is pressed. The viewer loads the current image plus one on each side. The viewer component is code-split and loaded on first open.
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

1. **Categories.** Reference A shows four body-style categories (sports and exotic, classics, muscle, truck and SUV). This plan keeps two, Classic and Modern, because `item.type` also drives the lot's atmosphere. More categories are easy once the backend has a separate body or category field. Is two enough for now?
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
| Avoid the tells (several of which appear in the references themselves): all-caps eyebrows, monospace data labels, middle-dot meta strings, arrows on links, single accented word, numbered markers on non-sequences, identical rounded shadowed cards, gradient washes | None are used. Numbered steps appear only in Sell, which is a real sequence. Meta facts are separated by spacing. |
| Warm cream + serif + terracotta is a generated default | The brief explicitly asks for ivory, charcoal and muted red, so the brief wins. The plan departs from the default where it is free to: the red is the references' deep signal red reserved for state rather than a clay accent, cards are white on a light ivory ground rather than cream on cream, primary actions are ink, and half the product runs on a dark stage. |
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
