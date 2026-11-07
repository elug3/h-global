# H-Global Backend Plan (draft v2)

> Status: draft for discussion, 2026-11-07. Nothing is built yet.
> Builds on: the live commerce plan (`docs/plan/livestream-commerce-plan.md` on branch
> `claude/h-global-livestream-plan-tj066k`) and the UI design draft in Figma
> ([H-Global UI — Design draft](https://www.figma.com/design/vNXgKEBXrTOleCbgKOqD4G)).
>
> v2: H-Global is **its own service, separate from Dupli1**, built as a **monolith**. Products are
> created on the spot during a stream; nothing has to be prepared in advance. v1 (built on Dupli1's
> microservices) is replaced.

## 1. Summary

H-Global is one Go program with one PostgreSQL database and one object-storage bucket. It serves:

- the **storefront** API (home, live page, product page, cart, checkout, account, 1:1 chat), and
- the **H-Global Studio** API (stream setup, live console, quick listing, orders, chat inbox),

and runs its background jobs (hold expiry, unpaid-order cancel, auto-hide after a stream, YouTube
polling) in the same process.

The main difference from Dupli1 is the product model. **Dupli1 needs brands, styles, colors, sizes
and SKU codes prepared before a product can exist. H-Global creates a product from a captured frame
and a price.** Options such as color and size are typed or picked from recently used values at
listing time, and everything else can be filled in after the stream.

```
 Storefront (web) ──┐                       ┌── H-Global Studio (staff web)
                    │ REST + SSE            │ REST + SSE
              ┌─────▼───────────────────────▼─────┐
              │          hglobal (one binary)     │
              │ ┌───────┐ ┌───────┐ ┌──────────┐  │
              │ │catalog│ │ live  │ │ checkout │  │     YouTube Data API
              │ └───────┘ └───────┘ └──────────┘  │ ──▶ (start time, viewers)
              │ ┌───────┐ ┌───────┐ ┌──────────┐  │
              │ │ order │ │payment│ │ support  │  │ ──▶ Payment provider
              │ └───────┘ └───────┘ └──────────┘  │     (card, virtual account)
              │ ┌───────┐ ┌───────┐ ┌──────────┐  │
              │ │account│ │ media │ │  notify  │  │ ──▶ Email / Telegram / Kakao
              │ └───────┘ └───────┘ └──────────┘  │
              │  platform: http · auth · sse hub · jobs · events
              └──────┬──────────────────────┬─────┘
                     ▼                      ▼
                PostgreSQL            Object storage (S3-compatible)
              (+ pgvector, Phase 2)   photos, captures, inspection videos
```

### Why a monolith fits

- **Publishing a live item is one database transaction.** The product, its options, its variants,
  their stock, the numbered stream item and the viewer event commit together or not at all. With
  separate services the same step needed a multi-call saga with retries.
- **One thing to deploy and back up.** The team runs one binary and one database.
- **The load is small for one process.** 1–2k viewers on a live feed, a few staff, bursts of orders
  when an item is shown on air.
- **Modules keep it splittable.** Each module owns its tables and is reached only through its
  service interface (section 3). If one part ever needs to scale alone, it can be cut out along that
  line.

## 2. Decisions to make first

The recommendation is in the second column.

| # | Question | Recommendation | Why it matters |
|---|---|---|---|
| D1 | Where are customers, and in what currency do they pay? | **Confirm.** The customs-number step in the meeting (order → inspection → customs number → shipping) is Korea's personal customs code (PCCC, `P` + 12 digits), used by buyers **in Korea** receiving goods from abroad. If so: KRW, Korean addresses. | Decides currency, payment provider, address format and shipping. The Figma checkout shows a US address, which should change if buyers are in Korea. |
| D2 | Payment provider | **Pick one Korean PG offering card and virtual-account (가상계좌) payments** with a deposit webhook. | The meeting's pain point is confirming bank deposits by hand. A virtual account per order reports the deposit automatically. The merchant name on the account also settles the "keep the owner's name off payments" concern. |
| D3 | How customers sign in | **Kakao login plus email.** Allow browsing and cart without an account, but ask for sign-in at checkout. | Live viewers arrive from YouTube on their phones, so one-tap sign-in matters. Sign-in at checkout gives chat, order history and the customs number a home. Guest checkout is possible later. |
| D4 | When is a one-of-a-kind item held for the buyer? | **At "Buy now" / start of checkout, for 10 minutes.** | Two viewers will press Buy on the same bag in the same second. Holding at Buy now means the one who gets it can finish the address and customs steps without losing it. The Figma checkout already shows this timer. |
| D5 | Quantity unknown on location | **Stock mode "until sold out"**: no count, sellable until the CS rep marks it sold out. Ask for a count whenever one is known. | It's our own model, so no fake ceiling is needed. The risk is overselling; the console shows orders arrive per item so the CS rep can stop in time. |
| D6 | Days on sale after a stream | **Setting per stream, default 3.** Items are hidden, not deleted, and can be kept permanently. | From the meeting; the number of days was not decided. |
| D7 | Hosting | **Confirm.** One container + PostgreSQL + S3-compatible storage behind HTTPS. Any VPS or managed Postgres works. | Must be decided before Phase 1 ends. |

## 3. Code layout

One Go module in this repository.

```
cmd/hglobal/main.go        # starts the HTTP server and the job runner (flags: --jobs, --http)
internal/
  platform/                # shared plumbing, no business rules
    config/  db/  httpx/  auth/  sse/  jobs/  events/  storage/  ids/  clock/
  account/                 # customers, staff, roles, sessions, addresses (incl. customs number)
  catalog/                 # products, option values, variants, stock, product media, search
  live/                    # shows, numbered show items, captures, YouTube, quick listing
  checkout/                # cart, holds, checkout
  order/                   # order lifecycle, inspection, customs, shipping, cancel/refund
  payment/                 # provider adapter, payment records, webhooks, refunds
  support/                 # 1:1 chat between a customer and staff
  notify/                  # email, Telegram (ops), Kakao (later)
migrations/                # SQL migrations, applied on startup
```

Each module has the same small set of files: `model.go` (types and rules), `service.go` (use
cases), `store.go` (the repository interface), `postgres.go` (its implementation), `http.go`
(handlers), plus tests.

Rules that keep the monolith tidy:

1. **A module owns its tables.** Other modules call its service, never its tables directly.
2. **Business rules live in `model.go` and `service.go`.** Handlers validate input and call the
   service; `postgres.go` only reads and writes.
3. **Cross-module work shares one transaction** through `platform/db.Tx` (unit of work). That is the
   reason for building a monolith, so use it: quick listing and checkout are single transactions.
4. **Modules announce changes as events** (`platform/events`): written to an `events` table in the
   same transaction, delivered to in-process subscribers and the live feed after commit.
5. **Every service is tested with an in-memory store**; Postgres tests run when `DATABASE_URL` is set.

## 4. Product model: spontaneous listing

### 4.1 What a product is

| Entity | Fields | Notes |
|---|---|---|
| **Product** | `id`, `title`, `description`, `price_won`, `status`, `source`, `specs` (key/value), `tags`, `details_complete`, `created_by` | Only a **photo and a price** are required. `title` defaults to `Live #9` and can be edited later. `source` is `live_capture` or `manual`. |
| **Option** | `product_id`, `name` (e.g. "Color", "Size"), `position` | Free text, per product. No master list. |
| **Option value** | `option_id`, `label` (e.g. "Brown", "Mini"), `hex1`, `hex2` | `hex1` draws the swatch; `hex2` makes it two-tone (Ivory / Black). Both optional. |
| **Variant** | `id`, `product_id`, option values, `price_override_won`, `stock_mode`, `stock_qty`, `held_qty`, `sold_qty`, `status` | One per sellable combination. A product with no options has one variant. |
| **Media** | `product_id`, `kind` (`photo`, `inspection_photo`, `inspection_video`), `object_key`, `position` | The inspection photos and video on the product page are product media, uploaded before or at sale. |

`status`: `draft` → `live` → `hidden` (after the stream's N days) → `archived`. `live` means
buyable and visible.

### 4.2 Keeping data consistent without masters

- **Suggestions, not masters.** When the CS rep types a color or size, Studio suggests the labels
  used most in the last 90 days (`GET /studio/catalog/suggestions?option=Color`). Picking one reuses
  its label and swatch color, so "Brown" stays one spelling with one hex.
- **Swatch colors are optional.** Without a hex, the swatch shows the label as a chip.
- **Details later.** `details_complete` is false until a title, description and specs are filled in.
  The after-stream list in Studio shows every product still missing them.

### 4.3 Stock and holds

```
available = stock_qty − held_qty − sold_qty        (stock_mode = counted)
available = unlimited until marked sold out          (stock_mode = until_sold_out)
```

- **Hold at Buy now (D4).** One conditional update takes the hold:
  `UPDATE variants SET held_qty = held_qty + $n WHERE id = $1 AND stock_qty − held_qty − sold_qty >= $n`.
  No row updated means sold out, so two buyers can never both get the last one.
- Holds live 10 minutes and are released by a job, by abandoning checkout, or by converting into a
  paid order (`held → sold`).
- **Cancel before shipping returns the stock; cancel after shipping does not.**

## 5. Live commerce (`live` module)

### 5.1 Show and show item

| Entity | Fields | Rules |
|---|---|---|
| **Show** | `id`, `youtube_video_id`, `title`, `scheduled_at`, `status` (`draft`, `scheduled`, `live`, `ended`), `started_at`, `ended_at`, `display_days_after`, `next_number`, `host_name`, `cs_user_id` | `started_at` comes from YouTube's actual start time when available. |
| **Show item** | `id`, `show_id`, `number`, `source` (`preselected`, `captured`), `product_id`, `pinned`, `broadcast_offset_sec`, `capture_id`, `keep_listed`, `hide_at`, `published_at`, `published_by` | `number` is unique per show and **never reused or renumbered once the show is live**. |
| **Capture** | `id`, `show_id`, `object_key`, `captured_at`, `broadcast_offset_sec`, `crop` | The raw frame. The cropped image becomes the product's first photo. |

- Preselected items (No. 1–8) can be reordered until the show starts; starting locks all numbers.
- A captured item takes `next_number` inside its publishing transaction.
- Ending a show sets `hide_at = ended_at + display_days_after` on every item not `keep_listed`.

### 5.2 Quick listing (the core flow)

Target: under 60 seconds from capture to a buyable item; the server's share under 1 second.

```
Studio                                  hglobal
  │ POST /studio/shows/{id}/captures ──▶ save frame to storage, compute broadcast offset
  │ ◀── capture (+ match suggestions, Phase 2)
  │ POST /studio/shows/{id}/items/quick (Idempotency-Key)
  │      { capture_id, crop, price, title?, options[], stock }
  │                                    ├─ save cropped photo to storage   (before the transaction)
  │                                    └─ one transaction:
  │                                         product (status live) + options + variants + stock
  │                                         + media + show item (number = next_number)
  │                                         + event live.item_published
  │ ◀── item                           after commit → SSE hub → every viewer
```

- **All or nothing.** If anything fails, the transaction rolls back and nothing is visible; Studio
  shows the error and the rep presses Publish again. A photo uploaded for a failed attempt is
  removed by a cleanup job.
- The `Idempotency-Key` makes a double click or a retry after a timeout return the same item.
- **Matched item** (Phase 2): if the rep picks an existing product, no product is created. The show
  item links to it, and the price can be changed in the same request.

### 5.3 Live feed to viewers (SSE)

- `GET /api/shows/{id}/events` is a Server-Sent Events stream. Each frame carries the **full item
  snapshot**, so viewers never need a follow-up request:
  `event: item` · `data: {"type":"live.item_published","item":{…}}`.
- Event types: `live.show_started`, `live.show_ended`, `live.item_published`, `live.item_updated`
  (price, title, photos), `live.item_pinned`, `live.item_unpinned`, `live.stock_changed`,
  `live.item_sold_out`.
- **Stock changes are pushed, not polled.** Holds, orders and cancels happen in the same process, so
  `catalog` publishes `stock_changed` whenever a variant on a live show changes, and the hub forwards
  it. Repeated changes to one variant are merged into one frame per second.
- `Last-Event-ID` replays from the `events` table (kept 24 h). Older than that, the server tells the
  client to reload `GET /api/shows/{id}`.
- Heartbeat every 15 s; the reverse proxy must not buffer `text/event-stream`.
- **One instance is enough** for the expected 1–2k viewers. If a second instance is ever needed,
  the hub fans out through PostgreSQL `LISTEN/NOTIFY`, so no message broker is required.
- `GET /api/shows/{id}` is served from an in-memory snapshot of each live show, kept current by the
  same events, so a burst of page loads when the host says "No. 5" does not hit the database.

### 5.4 YouTube

- YouTube Data API v3, `videos.list?part=liveStreamingDetails,snippet`: actual start time and
  concurrent viewers, polled every 30 s while a show is live (about 360 quota units for a 3-hour
  stream; the default daily quota is 10,000).
- Broadcast offset = capture time − actual start time − stream delay (a setting, default 10 s).
  Replay link: `youtube.com/watch?v={id}&t={offset}s`.
- API key stored as a secret, not in the repo. If YouTube is unreachable, the offset uses the Start
  press and the viewer count is hidden. Listing never waits on YouTube.

### 5.5 After the stream

- **Auto-hide job** (every minute): products whose show items passed `hide_at` and are not
  `keep_listed` move to `hidden`. Hidden products disappear from the site but keep their page for
  past orders and links.
- **Recap**: `GET /api/shows/{id}/recap` lists every item with its replay moment. Studio gives a
  share link and image for the KakaoTalk post. Sending through the Kakao API is Phase 3.
- **Completion list**: products from the stream with `details_complete = false`.

## 6. Orders, payment, customs

### 6.1 Order lifecycle

The meeting's flow (order → inspection → customs number → shipping) becomes the status machine:

```
pending_payment ──paid──▶ paid ──▶ inspecting ──▶ customs_check ──▶ shipped ──▶ delivered ──▶ completed
      │                    │            │               │
      └── 10 min unpaid ───┴── cancel (refund, stock returned) ──┘        cancel after shipping:
                                                                          refund, stock not returned
```

- **inspecting**: staff attach inspection photos/video for this order. The customer sees them in the
  order and on the product page if they are product media.
- **customs_check**: the order's customs number (PCCC) is checked before shipping. MVP: format check
  (`P` + 12 digits) plus a staff confirmation. Later: Korea Customs (UNI-PASS) lookup, once API
  access is confirmed.
- **shipped** requires a carrier and tracking number.
- Every change is an event: Telegram alerts for staff, email (later Kakao) to the customer, and a
  live update in Studio.

### 6.2 Checkout

1. **Buy now** (or checkout from cart) takes holds (4.3) and opens a checkout that expires with them.
2. The customer confirms the address and customs number (saved in their account for next time).
3. **Pay** creates the order `pending_payment` and a payment with the provider. The price is always
   recomputed on the server; the client never sends a total.
4. The provider's webhook marks the payment succeeded. The order becomes `paid` and holds become sold
   in one transaction. Webhooks are verified, idempotent, and reconciled by a job that asks the
   provider about anything still pending.
5. Virtual-account orders stay `pending_payment` until the deposit webhook arrives. They get a longer
   deadline (e.g. 24 h) while still holding stock, which needs a decision for one-of-a-kind items.

### 6.3 Payment module

- One `Provider` interface (`CreatePayment`, `VerifyWebhook`, `Refund`, `Lookup`) with one adapter
  for the chosen PG (D2) and a fake adapter for development and tests.
- Payments and refunds are stored with the provider's IDs; amounts are whole won.
- Refunds go through the provider first; the order changes only after the provider accepts.

## 7. Accounts, chat, notifications

- **account**: customers (Kakao / email sign-in), staff (roles `owner`, `manager`, `cs`), saved
  addresses with an optional customs number. Sessions are HttpOnly cookies held by the server, not
  tokens in the browser. Staff and customers use separate cookies and sign-in pages.
- **Roles** decide Studio access: `cs` can run the live console, list items, answer chat and move
  orders through inspection; `manager` adds refunds, stream setup and settings; `owner` adds staff
  management.
- **support**: one conversation per customer, messages with optional product or order references,
  SSE to both sides. Replaces KakaoTalk for 1:1 questions. The Studio Chat tab is its inbox.
- **notify**: Telegram alerts for staff (new order, paid, listing failed, stream started), email to
  customers (order paid, shipped, chat reply). Kakao messages are Phase 3.

## 8. API outline

Storefront under `/api/`, Studio under `/api/studio/` (staff session required).

| Area | Storefront | Studio |
|---|---|---|
| Live | `GET /shows/current`, `GET /shows/{id}`, `GET /shows/{id}/events` (SSE), `GET /shows/{id}/recap`, `GET /shows?status=ended` | `POST/PATCH /shows`, `POST /shows/{id}/start`, `/end`, `POST/DELETE /shows/{id}/items`, `PUT /shows/{id}/items/order`, `POST /shows/{id}/captures`, `POST /shows/{id}/items/quick`, `POST /items/{id}/pin`, `/unpin`, `/sold-out`, `GET /shows/{id}/console` |
| Catalog | `GET /products`, `GET /products/{id}` (incl. broadcasts and inspection media) | `PATCH /products/{id}`, options, variants, stock, media upload, `GET /catalog/suggestions`, `GET /catalog/incomplete` |
| Checkout | `GET/PUT /cart`, `POST /checkout` (Buy now / from cart, takes holds), `POST /checkout/{id}/pay` | — |
| Orders | `GET /orders`, `GET /orders/{id}`, `POST /orders/{id}/cancel` | `GET /orders`, `POST /orders/{id}/inspection`, `/customs-check`, `/ship`, `/deliver`, `/refund`, `GET /orders/events` (SSE) |
| Payment | `POST /payments/webhook/{provider}` (provider only) | `GET /payments`, refunds via orders |
| Account | `POST /auth/kakao`, `/auth/email`, `GET/PUT /me`, addresses | `POST /studio/auth/login`, staff management |
| Support | `GET/POST /support/messages`, `GET /support/events` (SSE) | `GET /support/conversations`, reply, close, `GET /support/events` (SSE) |

## 9. Data (one PostgreSQL database)

| Module | Tables |
|---|---|
| account | `customers`, `staff`, `sessions`, `addresses` |
| catalog | `products`, `product_options`, `option_values`, `variants`, `product_media` |
| live | `shows`, `show_items`, `captures` |
| checkout | `carts`, `cart_lines`, `checkouts`, `holds` |
| order | `orders`, `order_lines` (snapshot of title, options, price, photo), `order_events`, `shipments` |
| payment | `payments`, `refunds`, `webhook_receipts` |
| support | `conversations`, `messages` |
| platform | `events` (event log + SSE replay), `jobs`, `idempotency_keys` |
| Phase 2 | `image_embeddings` (pgvector) |

Migrations are plain SQL files in `migrations/`, applied in order on startup and tracked in a
`schema_migrations` table.

## 10. Non-functional

- **Load**: one process, 1–2k SSE connections, a few hundred requests per second at peaks. Live show
  reads come from memory; holds are single-row conditional updates.
- **Security**: staff routes need a staff session and role; public reads and SSE are rate-limited
  per IP; uploads accept images (5 MB) and videos (100 MB) only; payment webhooks are signature-checked.
- **Money**: whole won in integers, recomputed on the server, never trusted from the client.
- **Observability**: structured logs, error reporting (e.g. Sentry), and a log line per listing with
  capture-to-live time so the 60-second goal can be measured.
- **Backups**: nightly database dump and storage sync off the machine, with a tested restore.
- **Testing**: service tests with in-memory stores; Postgres tests in CI; one end-to-end test that
  creates a show, quick-lists an item, buys it with the fake provider and ships it.

## 11. Phases

| Phase | Scope |
|---|---|
| **0 — Decide** (with the 11/08 design review) | Answer D1–D7. Agree this plan. |
| **1 — MVP** (about 5–7 weeks) | Platform (config, DB, migrations, sessions, SSE hub, jobs, storage). Accounts and staff roles. Catalog with spontaneous products, options, variants, stock and media. Live: shows, preselected No. 1–8, capture, quick listing, pin, sold out, SSE feed, YouTube offsets, auto-hide. Checkout with holds; orders through shipping; one payment provider; Telegram alerts; email for paid and shipped. |
| **2 — Match and follow up** (about 3–4 weeks) | Image matching to existing products (pgvector); recap page and KakaoTalk share link; completion list; console stats per show; 1:1 chat; inspection media on the product page. |
| **3 — Later** | Kakao login if not in MVP, Kakao messages, stream reminders, YouTube chat, customs number lookup, reviews, analytics per item. |

Suggested build order inside Phase 1:

1. Platform skeleton, migrations, staff sign-in, roles.
2. Catalog: products, options, variants, stock, media upload.
3. Live: shows, preselected items, captures, quick listing in one transaction.
4. Events, SSE hub, in-memory show snapshot, stock-change push.
5. Checkout and holds; orders; payment provider adapter and webhook; hold and unpaid-order jobs.
6. YouTube adapter, auto-hide job, Telegram and email; end-to-end test.

## 12. Design follow-ups

- **Checkout**: if D1 confirms buyers are in Korea, use a Korean address and keep the customs
  number field. The 10-minute hold timer already matches D4.
- **Quick listing**: the Color and Size chips should come from recently used labels with a "+ Add"
  for new ones (4.2). Hex is optional, and two-tone uses two hex values on one option value, which
  matches the components board.
- **Stock**: the "Unknown — sell until marked sold out" toggle matches stock mode `until_sold_out`.
