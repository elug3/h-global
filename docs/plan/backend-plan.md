# H-Global Backend Plan (draft v1)

> Status: draft for discussion, 2026-11-06. Nothing is built yet.
> Builds on: the live commerce plan (`docs/plan/livestream-commerce-plan.md` on branch
> `claude/h-global-livestream-plan-tj066k`), the UI design draft in Figma
> ([H-Global UI — Design draft](https://www.figma.com/design/vNXgKEBXrTOleCbgKOqD4G)), and the
> Dupli1 backend as of `elug3/dupli1@787d27c`.

## 1. Summary

**Do not build a second commerce backend.** Dupli1 already runs, in production, almost everything a
store needs: accounts and permissions, a product catalog with SKUs, color swatches and stock,
cart, checkout with stock reservation, card payment, the order lifecycle with a required inspection
video before shipping, the overseas customs code (PCCC) on shipping addresses, 1:1 web chat, and
Telegram ops alerts.

What H-Global adds is **live commerce**, and that is one new service:

- **`live`** — streams (shows), the numbered items in each stream, quick listing from a captured
  frame, pinning "worn or held today", a real-time feed to viewers, YouTube timestamps, and hiding
  items N days after the stream.

`live` is written in Go to Dupli1's conventions and **calls Dupli1's product service** to create
the products it lists. It never stores prices, stock or orders itself. Checkout, payment and
shipping run through the existing Dupli1 services unchanged.

```
                    ┌──────────────── H-Global storefront (web) ────────────────┐
                    │  home · live page · product page · cart · checkout         │
                    └───────────────┬───────────────────────────┬───────────────┘
                                    │ REST + SSE (via BFF)      │ REST
┌── H-Global Studio (staff web) ──┐ │                           │
│ stream setup · live console     │ │                           │
└──────────────┬──────────────────┘ │                           │
               │ REST + SSE         │                           │
          ┌────▼────────────────────▼────┐       nginx gateway (routes.conf)
          │            live  (NEW)       │──────────────┐
          │ shows · items · captures     │              │ service account
          │ SSE hub · hide worker        │              ▼
          └──┬───────────┬───────────┬───┘   ┌─────── Dupli1 services (existing) ───────┐
             │           │           │       │ product: catalog, SKUs, images, stock     │
     Postgres│     NATS  │   S3      │       │ order: checkout, reservations, lifecycle  │
     `live`  │  live.*   │ captures  │       │ payment · cart · auth · profile (PCCC)    │
             ▼           ▼           ▼       │ support (1:1 chat) · notification (ops)   │
                                             └───────────────────────────────────────────┘
          YouTube Data API: stream start time, viewer count
```

## 2. Decisions to make first

These change the plan. The recommendation is in the second column.

| # | Question | Recommendation | Why it matters |
|---|---|---|---|
| D1 | Does H-Global share Dupli1's catalog, customers and orders, or run separately? | **Separate deployment of the same Dupli1 images** (own databases, own domain, own staff accounts), plus `live`. | One deployment would need a "channel" field threaded through product, order, cart and reports. A separate deployment needs no change to Dupli1's data model and keeps the two businesses' customers, stock and money apart. |
| D2 | Where are customers? | **Confirm.** Dupli1 is KRW-only and its PCCC field is for buyers *in Korea* receiving overseas-sourced goods. | If buyers are in Korea, everything fits. If buyers are abroad, currency, payment provider and shipping are all new work, and that is far bigger than live commerce. (The Figma checkout currently shows a US address; that should change if buyers are in Korea.) |
| D3 | Payment provider and merchant account | **Confirm.** Dupli1 supports NANO card (with a 10% card surcharge setting) and staff Bypass. Bank transfer is not built. | The meeting wants payments on the site, under a name that keeps the owner's personal name off payment paths. Whose merchant account is used is a business decision. |
| D4 | Where does the `live` code live? | **In the `dupli1` monorepo as `live/`.** The h-global repo keeps the plans, the design and the H-Global frontends. | `live` needs Dupli1's `shared` packages (permissions, events, outbox, auth middleware), its single Dockerfile, its gateway route table and its deploy files. Living elsewhere means copying all of that. |
| D5 | How is a brand-new item from the stream filed in the catalog? | **Under a placeholder brand (e.g. `HGL`, "H-Global Live"), one style per item.** | Dupli1 requires an existing brand and style before a product can be created, and SKU color/size codes cannot change later. Quick listing cannot wait for a real brand. A later step can create the proper catalog product and link it (see 6.7). |
| D6 | Quantity unknown on location | **Allow "sell until sold out": stock is set to a ceiling (e.g. 99) and the CS rep marks it sold out.** | Dupli1 treats zero stock as not for sale, so "unknown" has to become a number. The risk is selling more than exist; the CS rep sees orders arrive live and can stop it. |
| D7 | When is a one-of-a-kind item held for the buyer? | **MVP: keep Dupli1's rule.** Stock is reserved when the buyer presses Pay, then they have 5 minutes to pay. Later: hold at "Buy now". | Holding earlier is better for one-of-a-kind items but changes the order service. The Figma checkout shows the hold timer before the address step; with the MVP rule it belongs on the pay step and lasts 5 minutes. |

## 3. What Dupli1 already covers

| H-Global need (from the plan and design) | Dupli1 today | Gap |
|---|---|---|
| Staff and customer accounts, roles | `auth`: JWT, fine-grained permissions, bundles | New `live.*` permissions and a `live_cs` bundle |
| Product with photos, price, color and size options | `product`: parent + SKU variants, images, master dictionaries | Quick-listing flow (in `live`), placeholder brand (data) |
| Color swatches, including two-tone | Color master `hex`; two-tone via the SKU's `color2Code` (since 2026-11-05) | None. (The Figma components note says the master catalog holds two hex values; Dupli1 decided to put the second color on the SKU instead.) |
| Stock, sold out, no double-selling | `stock_items`, reservations at checkout, commit on ship | None for MVP |
| No "Official Price" shown | `officialPrice` is optional | Frontend only: do not render it |
| Cart, checkout, 5-minute payment window | `cart`, `order` checkout sessions | None |
| Card payment | `payment`: NANO card, Bypass | D3 |
| Order → inspection → customs → shipping | Order lifecycle; ship requires an inspection video; PCCC on the shipping address | PCCC is format-checked only, not verified with customs (6.8). A customs-cleared step is not a status. |
| Inspection photos/video on the product page | Inspection video is per **order** (after sale) | Product-level inspection media before sale is new (6.8) |
| 1:1 chat instead of KakaoTalk | `support` web chat (signed-in shoppers, SSE) + staff inbox | Reuse; the Studio Chat tab calls support's inbox API |
| Ops alerts | `notification` → Telegram | Add `live.*` alerts (publish failed, stream started) |
| Reviews | Not built (open in Dupli1's TODO) | Out of scope here |
| **Streams, numbered items, capture, live feed, auto-hide, replay links** | — | **The `live` service (section 4)** |

## 4. The `live` service

Same layout as every Dupli1 service (`live/cmd`, `live/pkg/{domain,service,ports,infra,handler,bootstrap}`),
stdlib `net/http`, PostgreSQL database `live`, NATS, S3-compatible storage for captured frames.
It falls back to in-memory repositories when `DUPLI1_LIVE_DB` is unset, like order and cart, so the
service tests need no database.

### 4.1 Domain

**Show** (one stream)

| Field | Notes |
|---|---|
| `id` | ULID |
| `youtube_video_id` | Parsed from the pasted link |
| `title`, `scheduled_at` | Shown on the home page and the schedule |
| `status` | `draft` → `scheduled` → `live` → `ended` |
| `started_at`, `ended_at` | `started_at` from YouTube's actual start time when available, else the Start press |
| `display_days_after` | Days items stay on sale after `ended_at`. Default 3. |
| `next_number` | Next item number to hand out. Only ever increases. |
| `host_name`, `cs_user_id` | Shown in Studio |

**ShowItem** (one numbered product in a stream)

| Field | Notes |
|---|---|
| `id`, `show_id` | |
| `number` | 1, 2, 3 … Unique per show. **Never reused or renumbered once the show is live.** |
| `source` | `preselected` or `captured` |
| `product_id`, `sku_ids` | The Dupli1 product and variants this item sells |
| `pinned` | "Worn or held today" |
| `broadcast_offset_sec` | Seconds from stream start, for the replay link |
| `capture_id` | The frame it was listed from, if captured |
| `state` | `publishing` → `live` → `hidden`; or `failed` (with `last_error`) |
| `stock_mode` | `counted` or `until_sold_out` (D6) |
| `keep_listed` | If true, never auto-hidden |
| `hide_at` | `ended_at + display_days_after`, set when the show ends |
| `published_at`, `published_by` | For the audit trail and the NEW badge |

**Capture**: `id`, `show_id`, object-storage key, `captured_at`, `broadcast_offset_sec`, crop box.

Rules that belong in the domain:

- Preselected items (No. 1–8) can be reordered only while the show is `draft` or `scheduled`.
  Starting the show locks every number.
- A captured item takes `next_number` in the same transaction that creates it, so two CS reps
  listing at once never get the same number.
- `hide_at` is computed once, at end of show. Changing `display_days_after` afterwards recomputes it
  for items not yet hidden.
- An item becomes visible to viewers only when it reaches `live`. A half-finished listing is never
  shown (4.3).

### 4.2 APIs

Canonical prefix `/api/v1/live/`, routed in `api/gateway/routes.conf`.

**Public (storefront, no sign-in)**

| Method | Path | Returns |
|---|---|---|
| GET | `/shows/current` | The live show, else the next scheduled one (home banner) |
| GET | `/shows?status=ended` | Recent streams (recaps) |
| GET | `/shows/{id}` | Show + items: number, pinned, NEW flag, and a product summary (name, price, swatches, first image, `availableQty`) |
| GET | `/shows/{id}/events` | **SSE** feed of changes to that show (4.4) |
| GET | `/shows/{id}/recap` | Ended show: all items + replay link, for the KakaoTalk follow-up |
| GET | `/products/{productId}/broadcasts` | Where a product was shown: show, time, replay URL `youtube.com/watch?v={id}&t={offset}s` (product page "Broadcast on…") |

**Studio (staff; permission in brackets)**

| Method | Path | Does |
|---|---|---|
| POST / PATCH | `/shows`, `/shows/{id}` | Create and edit a stream (`live.show.manage`) |
| POST | `/shows/{id}/start`, `/shows/{id}/end` | Go live / end (`live.show.manage`) |
| POST / DELETE | `/shows/{id}/items` | Add or remove a preselected catalog product (`live.show.manage`) |
| PUT | `/shows/{id}/items/order` | Reorder No. 1–8 before going live (`live.show.manage`) |
| POST | `/shows/{id}/captures` | Upload a captured frame; returns the capture and, from Phase 2, catalog match suggestions (`live.item.publish`) |
| POST | `/shows/{id}/items/quick` | **Quick listing**: capture + crop + price + options → numbered live item (`live.item.publish`). Takes an `Idempotency-Key`. |
| PATCH | `/items/{id}` | Edit price, stock, name, photos after publishing (`live.item.publish`) |
| POST | `/items/{id}/pin`, `/unpin`, `/sold-out`, `/retry` | Pin "worn or held today", mark sold out, retry a failed listing (`live.item.publish`) |
| GET | `/shows/{id}/console` | Console stats: sold / stock per item, orders and revenue for the show's SKUs, YouTube viewer count (`live.show.read`) |

Studio uses the same SSE feed, and order's existing staff stream (`GET /api/v1/orders/events`) for
orders as they arrive. The Chat tab uses support's existing inbox API.

### 4.3 Quick listing (the core flow)

Goal: under 60 seconds from capture to a buyable item, with the backend's share under 2 seconds.

```
Studio                       live                                   Dupli1 product
  │ POST /captures (frame) ───▶ store frame, compute offset
  │ ◀── capture id (+ matches, Phase 2)
  │ POST /items/quick ────────▶ item = publishing, number = next_number   (1 transaction)
  │                             ├─ new item:  create style ─────────────▶ masters
  │                             │             create parent (status draft, price) ─▶ products
  │                             │             create SKUs (color × size) ───────────▶ variants
  │                             │             upload cropped image ─────────────────▶ images
  │                             │             set stock (count, or ceiling) ────────▶ inventory
  │                             ├─ matched item: link the existing product (no product writes)
  │                             ├─ activate product (status active) ─────────────────▶ products
  │                             └─ item = live; write live.item_published to outbox (1 transaction)
  │ ◀── item                    outbox → NATS → every replica's SSE hub → viewers
```

- **Nothing half-made is visible.** The product is created as `draft` and set `active` as the last
  product call. The item turns `live` after that.
- **Each step is recorded on the item**, so `POST /items/{id}/retry` resumes where it stopped instead
  of creating a second product. The `Idempotency-Key` stops a double click from listing twice.
- **Errors are shown, not hidden.** If product is down, the item goes `failed` with the reason. The
  console shows it, and the number stays taken (numbers are never reused).
- `live` writes to product with its own **service account** (`dupli1-live`), so background jobs work
  too. The staff member who pressed Publish is recorded on the item.

### 4.4 Real-time feed to viewers

Dupli1's admin order stream already solves this. `live` copies the pattern:

- **SSE, not WebSocket.** It goes through the BFF and nginx like any GET, and the browser
  reconnects by itself.
- State changes write an event row in the same transaction (transactional outbox,
  `shared/pkg/outbox`). A drain worker publishes to NATS `live.*`. Every `live` replica holds a
  broadcast subscription and relays to the streams it holds.
- Each frame carries the **full item snapshot**, so clients never need a follow-up GET:
  `event: item` / `data: {"type":"live.item_published","item":{…}}`.
- Event types: `live.show_started`, `live.show_ended`, `live.item_published`, `live.item_updated`
  (price, name, photos, stock), `live.item_pinned`, `live.item_unpinned`, `live.item_sold_out`.
  `live.item_failed` goes to NATS (for Studio and ops alerts) but is never sent to viewers.
- `Last-Event-ID` replays from the event table, kept for 24 hours. Older than that, the server
  tells the client to reload `/shows/{id}`.
- Heartbeat every 15 s, so Cloudflare Tunnel and nginx keep the connection open. The nginx location
  needs `proxy_buffering off` and a long read timeout.
- **Stock**: product publishes no stock events today. For MVP, `live` polls availability for the
  items of live shows every 3 s with the batch lookup `GET /api/v1/products/variants?sku_ids=`
  (≤ 50 per call) and emits `live.item_updated` only when a number changes. Later, a
  `product.stock_changed` event in Dupli1 can replace the poll.

### 4.5 YouTube

- YouTube Data API v3, `videos.list?part=liveStreamingDetails,snippet`. It returns the actual start
  time and the concurrent viewer count. Polled every 30 s during a live show (about 360 quota units
  for a 3-hour stream; the daily default is 10,000).
- Broadcast offset = capture time − actual start time − stream delay (a setting, default 10 s).
- Needs an API key for `www.googleapis.com`, stored as a secret, never in the repo.
- If the API is unreachable, the offset uses the Start press time and viewer count is hidden.
  Listing never waits on YouTube.

### 4.6 After the stream

- **Hide worker** (every minute): items whose `hide_at` has passed and that are not `keep_listed`
  are set to `archived` in product, then `hidden` in `live`. This is idempotent and safe to re-run.
  Archived, not deleted: past orders and links keep working.
- **Recap**: `/shows/{id}/recap` is public once the show ends. The Studio "After the stream" screen
  gives a share link and image for the KakaoTalk post. Sending through the Kakao API is Phase 3.
- **Completion list**: items still missing a description or real name. Editing them updates the
  product through `live` (or the Dupli1 product form directly).

### 4.7 Data (PostgreSQL `live`)

| Table | Key columns |
|---|---|
| `shows` | `id`, `youtube_video_id`, `title`, `status`, `scheduled_at`, `started_at`, `ended_at`, `display_days_after`, `next_number` |
| `show_items` | `id`, `show_id`, `number` (unique with `show_id`), `source`, `product_id`, `sku_ids`, `pinned`, `state`, `last_error`, `publish_steps` (JSONB), `stock_mode`, `keep_listed`, `broadcast_offset_sec`, `hide_at`, `published_at`, `published_by`, `idempotency_key` |
| `captures` | `id`, `show_id`, `object_key`, `captured_at`, `broadcast_offset_sec`, `crop` |
| `live_events` | `id` (monotonic), `show_id`, `type`, `payload`, `created_at`, `published_at` — outbox and replay log |
| `image_embeddings` (Phase 2) | `product_id`, `sku_id`, `image_url`, `embedding` (pgvector) |

Schema is migrated inline on startup, as every Dupli1 service does.

## 5. Changes needed in the `dupli1` repo

Small and additive. None of them change behavior for Dupli1's own storefront.

| Change | Where |
|---|---|
| `live/` service module, Compose service, VENUS deploy entry | new directory, `docker-compose.yml`, `deploy/venus/` |
| Gateway route `/api/v1/live/` with an SSE-friendly location | `api/gateway/routes.conf` + each wrapper's `$gw_live` upstream |
| Permissions `live.show.read`, `live.show.manage`, `live.item.publish`; bundle `live_cs` = those + `support.read`, `support.reply`, `order.read.all` | `shared/pkg/permissions` |
| Service account `dupli1-live` (`product.create`, `product.update`, `product.variant.create`, `product.variant.update`, `product.image.upload`, `product.master.write`, `inventory.stock.write`), API key from `DUPLI1_LIVE_SERVICE_API_KEY` | `shared/pkg/serviceaccount`, auth seeding |
| Event subjects `live.*` and payload structs | `shared/pkg/events` |
| `notification` subscribes `live.item_failed` and `live.show_started` (new alert class `alert_live`) | `notification` |
| Placeholder brand `HGL` | data, through the master API (no code) |
| Later: `product.stock_changed` event; hold stock at "Buy now" (D7); product-level inspection media (6.8) | `product`, `order` |

## 6. Details and open points

1. **Load.** Expect 1–2k viewers on SSE during a stream. One Go process holds that easily. The
   public show read is served from an in-memory snapshot per live show, updated by its own events,
   so a burst of page loads does not hit Postgres or product.
2. **Security.** Public reads and SSE are rate-limited per IP. Capture uploads: images only, 5 MB
   maximum. Every Studio route checks permissions, and staff IDs are recorded on writes.
3. **Prices.** `live` sends the price to product and never computes totals. Order prices checkout
   server-side, as today.
4. **Matched items** (Phase 2) link an existing product. Changing the price on air changes the
   catalog price. Decide whether that is wanted or whether a stream price needs its own override.
5. **Observability.** `sentrymon` like every service, plus a log line per listing with the
   capture-to-live time, so the 60-second goal can be measured.
6. **Tests.** Service-layer unit tests with in-memory fakes for product, YouTube and storage;
   Postgres tests run when `POSTGRES_URL` is set; a smoke script that creates a show, quick-lists an
   item and buys it through the existing money path (`scripts/smoke-money-path.sh`).
7. **Promoting a live item to the real catalog.** Because SKU codes are fixed, filing a stream item
   under its real brand means creating a proper product and archiving the placeholder one, with the
   live item pointing to the new product. Confirm whether brand can change on an existing product
   before building this. It is Phase 2.
8. **Customs and inspection.** PCCC (`P` + 12 digits) is already on Dupli1 addresses and checkout,
   checked for format only. The Figma "Verified" state needs a real check: Korea Customs (UNI-PASS)
   offers a PCCC lookup, but API access has to be confirmed. Product-page inspection media before
   sale does not exist yet. The simplest version is a tagged image group on the product.
9. **Image matching** (Phase 2). An image embedding for every catalog photo, stored with pgvector,
   with nearest neighbours for each capture. Hosting the model on VENUS versus calling an external
   API is open. `live` already has the `ImageMatcher` port with a no-op adapter in MVP, so the flow
   does not change when it arrives.

## 7. Phases

| Phase | Backend scope | Depends on |
|---|---|---|
| **0 — Decide** (with the 11/08 design review) | Answer D1–D7. Agree this plan. | — |
| **1 — MVP** (about 4–6 weeks) | `live` service: shows (create, start, end), preselected No. 1–8, capture upload, quick listing of **new** items, pin, sold out, edit, retry; public show read + SSE; YouTube start time and replay links; hide worker. Dupli1 changes in section 5. H-Global deployment of Dupli1 (D1). | D1, D2, D3, D4 |
| **2 — Match and follow up** (about 3–4 weeks) | Image matching for existing products; recap page and KakaoTalk share link; completion list; console stats (orders and revenue per show); viewer count; promote-to-catalog (6.7). | Phase 1 |
| **3 — Later** | Stock hold at "Buy now"; `product.stock_changed` event; stream reminders; Kakao API sending; YouTube chat; per-item analytics; PCCC verification; product-level inspection media. | — |

Suggested build order inside Phase 1:

1. Skeleton `live` service, Postgres schema, in-memory fakes, gateway route, permissions, service account.
2. Shows and preselected items (Studio setup screen works end to end).
3. Captures and quick listing for new items, with retry and idempotency.
4. Outbox → NATS → SSE hub; public show read; stock poll.
5. YouTube adapter; broadcast offsets; `/products/{id}/broadcasts`.
6. Hide worker; end-of-show; smoke test through checkout and payment.

## 8. Design follow-ups from this plan

- Checkout: Korean address and the PCCC field (if D2 confirms buyers are in Korea). The hold timer
  belongs on the pay step and lasts 5 minutes under D7's MVP rule.
- Components: two-tone swatches come from the SKU's second color (`color2Code`), not from two values
  in the master catalog.
- Card payments may carry Dupli1's card surcharge. If H-Global keeps it, checkout should show it.
