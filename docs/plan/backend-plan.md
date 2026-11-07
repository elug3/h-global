# H-Global Backend Plan (draft v7)

> Status: 2026-11-07. Phase 1 core of hg-server is built (see "Implementation status" below and
> [hg-server/README.md](../../hg-server/README.md)). Covers hg-server and how storefront and hg-studio
> connect to it.
> Builds on: the live commerce plan (`docs/plan/livestream-commerce-plan.md` on branch
> `claude/h-global-livestream-plan-tj066k`) and the UI design draft in Figma
> ([H-Global UI — Design draft](https://www.figma.com/design/vNXgKEBXrTOleCbgKOqD4G)).
>
> v2: H-Global is **its own service, separate from Dupli1**, built as a **monolith**. Products are
> created on the spot during a stream; nothing has to be prepared in advance. v1 (built on Dupli1's
> microservices) is replaced.
> v3: the system is **three programs**: `hg-server`, `storefront` and `hg-studio` (section 3).
> v4: the broadcast goes **through hg-server's media relay to YouTube**, so frames reach the agent
> and the CS rep before YouTube (5.5, 5.6).
> v7: decisions D1, D2 (on hold), D3 (email only for now) and D7 recorded; sign-in (7.1) and hosting (7.2) added.
> v5: the agent runs as its own process, **hg-agent** (`hg-server --agent`), which perceives;
> hg-server keeps every decision (5.6).

## Implementation status (2026-11-07)

hg-server's Phase 1 core is in [`hg-server/`](../../hg-server/README.md), with the HTTP contract in
[`api/openapi.yaml`](../../api/openapi.yaml).

**Built:** accounts and staff roles; spontaneous products with options, variants, stock, holds and
media; shows with preselected items, captures (uploaded), one-transaction quick listing, pinning,
sold out, keep listed and auto-hide; the viewer feed (SSE with replay); broadcast links and recaps;
checkout with 10-minute holds and the customs code; the order lifecycle through inspection, customs
check, shipping and delivery, with cancel and refund; the payment provider interface with a fake
provider, verified and idempotent webhooks, and automatic refund of late payments; the staff order
feed; the four jobs.

**Differences from this plan:**

- **PostgreSQL 18** is the database. IDs are UUIDv7 (PostgreSQL 18's `uuidv7()` as column
  default), and available stock is a virtual generated column.
- **Tests run against a real PostgreSQL 18** instead of in-memory stores (rule 5 in 3.2).
- Each module's SQL lives in its `postgres.go`, used directly by its service; interfaces exist only
  where an import cycle or an outside system needs one (`payment.OrderHook`, `payment.Provider`,
  `storage.Storage`).
- Stock and sold-out changes reach viewers as `live.item_updated` (5.3). There is no separate
  `stock_changed` event.
- `GET /api/shows/{id}` reads from the database; the in-memory snapshot (5.3) is not built yet.
- Staff watch a show at `/studio-api/shows/{id}/events` and orders at `/studio-api/orders/events`.
- Jobs run one at a time from one scheduler (running them in parallel could exhaust the connection
  pool).

**Broadcast relay prototype (2026-11-07):** built with ffmpeg instead of a separate media server,
with an MJPEG preview instead of WebRTC. Frames reach hg-server about 0.5 s after the camera in
local tests; see [docs/prototype/relay.md](../prototype/relay.md) for results, differences from
5.5 and how to measure on H-Global's server.

**hg-studio (2026-11-07):** built in Korean ([hg-studio/README.md](../../hg-studio/README.md)):
sign-in, stream setup, the live console, orders (list, detail, inspection to delivery, cancel and
refund), products (list, editing, photos, inspection media, stock per option), the after-stream
screen (sales, keep listed, sale period, completion list, KakaoTalk recap text) and staff
management. Tested end to end in a browser against hg-server and the relay. For these screens
hg-server gained order search, order counts per status and sales per stream (5.7).

**Not built yet:** YouTube Data API, hg-agent (5.6),
notifications, a real payment provider (D2), Kakao sign-in (D3), saved addresses, cart, 1:1 chat,
the internal API, storefront, the recap share image (5.7), and editing a product's options after
it is created.

## 1. Summary

H-Global is three programs:

| Program | What it is | Who uses it |
|---|---|---|
| **hg-server** | The Go monolith: all business rules, one PostgreSQL database, one object-storage bucket, background jobs. The only program that touches the database. | storefront and hg-studio, through its HTTP API |
| **storefront** | The customer website: home, live page, product page, cart, checkout, account, 1:1 chat. Mobile first. | Viewers and buyers |
| **hg-studio** | The staff web app, shown to users as **H-Global Studio**: stream setup, live console, quick listing, orders, chat inbox. Desktop only. | Host, CS reps, managers |

hg-server also runs its background jobs (hold expiry, unpaid-order cancel, auto-hide after a stream,
YouTube polling) in the same process.

Two supporting processes run beside the three programs:

- **media-relay** receives the broadcast, copies it to YouTube and taps frames and audio (5.5).
  It is an existing media server, not our code.
- **hg-agent** watches the frames and drafts listing proposals (5.6). It is hg-server's own code
  and image started in a different mode (`hg-server --agent`). It never touches the database.

The main difference from Dupli1 is the product model. **Dupli1 needs brands, styles, colors, sizes
and SKU codes prepared before a product can exist. H-Global creates a product from a captured frame
and a price.** Options such as color and size are typed or picked from recently used values at
listing time, and everything else can be filled in after the stream.

```
   storefront ───────┐                     ┌─────── hg-studio
  (customer web)     │ /api  REST + SSE    │ /studio-api  REST + SSE
                     │                     │   (H-Global Studio, staff web)
              ┌──────▼─────────────────────▼──────┐
              │        hg-server (one binary)     │
              │ ┌───────┐ ┌───────┐ ┌──────────┐  │
              │ │catalog│ │ live  │ │ checkout │  │     YouTube Data API
              │ └───────┘ └───────┘ └──────────┘  │ ──▶ (viewer count)
              │ ┌───────┐ ┌───────┐ ┌──────────┐  │
              │ │ order │ │payment│ │ support  │  │ ──▶ Payment provider
              │ └───────┘ └───────┘ └──────────┘  │     (card, virtual account)
              │ ┌───────┐ ┌───────┐ ┌──────────┐  │
              │ │account│ │ jobs  │ │  notify  │  │ ──▶ Email / Telegram / Kakao
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
| D1 | Where are customers, and in what currency do they pay? | **Decided 2026-11-07: buyers are in Korea and pay in won (KRW).** | Korean addresses with postcode search, Korean phone numbers, and the personal customs code (PCCC) for goods bought abroad. The Figma checkout's US address must change. |
| D2 | Payment provider | **On hold until the business operator's permit is granted.** hg-server keeps the fake provider behind `payment.Provider`. When the permit arrives, choose a Korean PG with card and virtual-account (가상계좌) payments and a deposit webhook; the adapter is about a week of work. | Real sales cannot start before this. A virtual account per order removes the manual deposit checking from the meeting, and the merchant name keeps the owner's name off payments. |
| D3 | How customers sign in | **Decided 2026-11-07: email only for now** (built). Kakao login and phone number (SMS code) come later, probably after the business permit (7.1). Browsing needs no account; checkout does. | Email works without any outside service. Kakao and SMS both need provider accounts, and SMS sender registration and Kakao's phone-number access need business registration. |
| D4 | When is a one-of-a-kind item held for the buyer? | **At "Buy now" / start of checkout, for 10 minutes.** | Two viewers will press Buy on the same bag in the same second. Holding at Buy now means the one who gets it can finish the address and customs steps without losing it. The Figma checkout already shows this timer. |
| D5 | Quantity unknown on location | **Stock mode "until sold out"**: no count, sellable until the CS rep marks it sold out. Ask for a count whenever one is known. | It's our own model, so no fake ceiling is needed. The risk is overselling; the console shows orders arrive per item so the CS rep can stop in time. |
| D6 | Days on sale after a stream | **Setting per stream, default 3.** Items are hidden, not deleted, and can be kept permanently. | From the meeting; the number of days was not decided. |
| D7 | Hosting | **Decided 2026-11-07: H-Global's own server.** Docker Compose on that machine: the apps, hg-server, PostgreSQL 18, the media relay, and a TLS proxy (7.2). Files on local disk, backed up off the machine. | Needs the server's details (7.2) before the first deployment. Upload bandwidth matters for the relay. |
| D8 | Web stack for storefront and hg-studio | **TypeScript + React Router**, the stack Dupli1's web apps already use: storefront server-rendered, hg-studio as a single-page app. | The team already knows it. The storefront needs server rendering so product and recap links show a title, price and photo when shared on KakaoTalk; hg-studio sits behind a login and needs none. |
| D9 | How the broadcast reaches YouTube | **Through hg-server's media relay** (5.5): one upload from location; the relay copies it to YouTube and taps frames and audio. A saved OBS profile that streams straight to YouTube is the fallback. | Frames arrive seconds before YouTube shows them and at full quality. The cost is that the relay must be up for YouTube to get the stream. Sending two uploads from the camera (one to YouTube, one to us) avoids that but doubles the uplink, which is often weak on location. |
| D10 | May the agent publish by itself? | **No. It prepares a listing; a CS rep approves it with one tap** (5.6). Revisit after measuring accuracy. | A misheard price is a money error. Approval still cuts the rep's work from about 60 seconds to a few seconds per item. |

## 3. Programs and code layout

### 3.1 Repository layout

All three programs live in this repository, so a feature that touches the API and both apps is one
pull request.

```
h-global/
  hg-server/            # Go module: the monolith
  storefront/           # customer web app (TypeScript, React Router, server-rendered)
  hg-studio/            # staff web app (TypeScript, React Router, single-page app)
  api/openapi.yaml      # the HTTP contract between hg-server and the two apps
  deploy/               # docker-compose, reverse proxy config, environment examples
  docs/
```

`api/openapi.yaml` is the single source of the API. hg-server's handlers are checked against it in
tests, and both apps generate their TypeScript client from it, so a changed field breaks the build
instead of breaking the page.

### 3.2 hg-server layout

Inside `hg-server/`:

```
cmd/hg-server/main.go      # one binary, three modes: --http, --jobs (default: both), --agent
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
  agent/                   # hg-agent only: frame sampling, model calls, speech-to-text, matching;
                           # talks to hg-server through internalclient/, never imports other modules
  internalclient/          # typed client for /internal-api, used by agent/
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
5. **Services are tested against a real PostgreSQL 18**, one fresh schema per test
   (`HG_TEST_DATABASE_URL`); fakes are used only for things outside the database (payment provider,
   file storage).

### 3.3 How the programs talk

hg-server exposes **one API surface per caller**, so a customer session can never reach a staff
route even by mistake:

| Surface | Used by | Session | Contents |
|---|---|---|---|
| `/api/…` | storefront | customer cookie `hg_session` | shows, products, cart, checkout, orders, account, chat, SSE feeds |
| `/studio-api/…` | hg-studio | staff cookie `hg_staff_session` + role check | stream setup, live console, quick listing, catalog editing, orders, refunds, chat inbox, staff SSE |
| `/hooks/…` | payment provider | signature check | payment and deposit webhooks |
| `/internal-api/…` | hg-agent | service token, private network only | frames, detections, drafts, similar products, agent heartbeat (5.6) |

- **No tokens in the browser.** Each app serves its pages and **proxies its own API path to
  hg-server** on the same origin (storefront `/api/*`, hg-studio `/studio-api/*`). Cookies stay
  first-party and HttpOnly, and no cross-origin (CORS) setup is needed.
- **SSE goes through the same proxy.** The app's proxy must pass `text/event-stream` through without
  buffering.
- **storefront renders on the server** for the home, live, product and recap pages, calling
  hg-server from its own server, so shared links carry Open Graph tags (title, price, photo).
- **hg-studio** is a single-page app served as static files by a small server that also does the
  proxying. It no longer captures the YouTube tab itself: the live preview and the frames come from
  hg-server's broadcast relay (5.5), so it works in any modern browser.
- The apps hold **no business rules**. Prices, stock, numbering and order status are decided by
  hg-server only.

### 3.4 Deployment

| Container | Public address (example) | Talks to |
|---|---|---|
| `storefront` | `hglobal.example` | hg-server (internal network) |
| `hg-studio` | `studio.hglobal.example` | hg-server (internal network) |
| `hg-server` | not public, except `/hooks/*` for the payment provider | PostgreSQL, object storage, YouTube, payment provider, email, Telegram, media relay |
| `media-relay` | `ingest.hglobal.example` (SRT/RTMP port only) | receives the broadcast, pushes it to YouTube, gives hg-server frames and the staff preview, gives hg-agent audio (5.5) |
| `hg-agent` | not public | hg-server `/internal-api` (private network), media-relay (audio), model APIs, speech-to-text (5.6) |

- One reverse proxy (TLS) in front routes the two domains and `/hooks/*`.
- The media relay is reached directly by the broadcaster on its ingest port. It is not behind the
  web proxy.
- hg-server, its job runner and hg-agent are one image started in different modes. `--http` and
  `--jobs` run in one process by default and can be split later; `--agent` always runs as its own
  container, so it can be restarted, scaled down or switched off without touching the shop.
- hg-studio can additionally be limited to staff networks or put behind an access gateway, since
  only staff use it.

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
  used most in the last 90 days (`GET /studio-api/catalog/suggestions?option=Color`). Picking one reuses
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
| **Show** | `id`, `youtube_video_id`, `title`, `scheduled_at`, `status` (`draft`, `scheduled`, `live`, `ended`), `started_at`, `ended_at`, `display_days_after`, `next_number`, `host_name`, `cs_user_id`, `ingest_key_hash`, `relay_state` | `started_at` is when the relay first received the stream (5.5). |
| **Show item** | `id`, `show_id`, `number`, `source` (`preselected`, `captured`), `product_id`, `pinned`, `broadcast_offset_sec`, `capture_id`, `keep_listed`, `hide_at`, `published_at`, `published_by` | `number` is unique per show and **never reused or renumbered once the show is live**. |
| **Capture** | `id`, `show_id`, `object_key`, `stream_pts_ms`, `broadcast_offset_sec`, `crop`, `source` (`relay`, `upload`) | A full-resolution frame from the relay's ring buffer (or an uploaded photo). The cropped image becomes the product's first photo. |

- Preselected items (No. 1–8) can be reordered until the show starts; starting locks all numbers.
- A captured item takes `next_number` inside its publishing transaction.
- Ending a show sets `hide_at = ended_at + display_days_after` on every item not `keep_listed`.

### 5.2 Quick listing (the core flow)

Target: under 60 seconds from capture to a buyable item; the server's share under 1 second.

```
hg-studio                               hg-server
  │ POST /studio-api/shows/{id}/captures { at } ▶ take that frame from the relay's ring buffer,
  │                                    save it to storage, offset = its stream timestamp
  │ ◀── capture (+ match suggestions, Phase 2)
  │ POST /studio-api/shows/{id}/items/quick (Idempotency-Key)
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
- Event types: `live.show_started`, `live.show_ended`, `live.show_updated`, `live.item_published`,
  `live.item_updated` (price, title, photos, stock, sold out), `live.item_pinned`,
  `live.item_unpinned`, `live.item_removed`, `live.items_reordered`.
- **Stock changes are pushed, not polled.** Holds, orders and cancels happen in the same process, so
  every catalog change to a featured product publishes `live.item_updated` with the full item.
  Merging repeated changes into one frame per second is not built yet.
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
- The **video itself reaches YouTube through hg-server's relay** (5.5). The Data API is used only
  for the viewer count and to confirm the stream is live.
- Broadcast offset = the frame's timestamp in the relayed stream (5.5), which is exact. The YouTube
  start time is only a fallback. Replay link: `youtube.com/watch?v={id}&t={offset}s`.
- API key stored as a secret, not in the repo. If YouTube's API is unreachable, the viewer count is
  hidden. Listing never waits on YouTube.

### 5.5 Broadcast relay: frames before YouTube

**Problem.** Taking frames from YouTube is too slow. YouTube adds roughly 5–30 seconds of delay
(depending on the stream's latency setting) and recompresses the picture, so a listing starts late
and from a blurry frame.

**Design.** The broadcaster sends the stream to **hg-server's relay** instead of to YouTube. The
relay passes the same stream on to YouTube and, at the same moment, hands frames to hg-server (for
the CS rep and hg-agent) and audio to hg-agent.

```
 camera / OBS on location
        │ SRT (or RTMP), one upload, per-show ingest key
        ▼
 ┌──────────── media-relay ────────────┐
 │ receive ──┬─▶ copy to YouTube (RTMPS, no re-encode) ──▶ YouTube ──▶ viewers (5–30 s later)
 │           ├─▶ frame tap: 2 fps, full resolution ──────▶ hg-server ring buffer (last 120 s)
 │           ├─▶ audio tap: 16 kHz mono ─────────────────▶ hg-agent: speech-to-text ──▶ transcript
 │           └─▶ low-latency preview (WebRTC) ───────────▶ hg-studio live console
 └─────────────────────────────────────┘
```

- **Copy, don't re-encode.** The relay forwards the original H.264/AAC stream to YouTube unchanged.
  That uses almost no CPU, keeps the original quality, and adds well under a second to what YouTube
  already adds.
- **Frames are seconds earlier and sharper.** A frame reaches hg-server about 0.5–1 s after the
  camera, at the original bitrate, so the agent (5.6) or the CS rep works from the best picture.
- **Exact timestamps.** Every frame carries its position in the stream, so the replay link
  (`&t=`) is exact instead of estimated.
- **hg-studio gets a near-real-time preview** from the relay instead of sharing a browser tab.
  **Capture** picks a frame from hg-server's ring buffer, and the rep can step back through the last
  few seconds to find a sharp one.
- **Ingest keys per show.** Stream setup in hg-studio shows the ingest URL and a one-time key for
  that show. The relay asks hg-server to authorise each connection, and the YouTube stream key never
  leaves the server.
- **SRT preferred over RTMP** from location (markets, warehouses, mobile uplinks), because it
  recovers lost packets instead of stalling. RTMP stays available for simple setups.
- **Recording (optional):** the relay can also save the original stream to object storage, for
  clips and for checking a disputed price later.

**Build vs buy.** hg-server stays the one place for business logic, but the video plumbing is a
proven media server running beside it as its own container (for example MediaMTX, with ffmpeg doing
the copy to YouTube and the frame decode). hg-server controls it: it starts and stops the YouTube
push for a show, authorises ingest keys, reads frames and audio, and watches its health. Writing
video handling into the Go process is possible but slower to get right, and a crash there would take
the web shop down with the broadcast.

**Failure and fallback.**

| Failure | What happens | Mitigation |
|---|---|---|
| YouTube push drops | Ingest and frames continue; YouTube shows "stream offline" | The relay reconnects automatically; hg-studio shows a red banner; Telegram alert |
| Relay or its machine goes down | The broadcast to YouTube stops | A saved OBS profile that streams straight to YouTube, one click to switch; listing falls back to manual capture. This is the cost of a single upload (D9). |
| Upload from location is weak | Same as streaming straight to YouTube | SRT reduces stalls. Set the camera bitrate to what the uplink holds. |

**Cost.** No re-encoding, so the relay needs little CPU. The server carries the stream twice (in and
out): at 6 Mbps, about 8 GB each way for a 3-hour stream. Check the hosting provider's egress
pricing (D7).

### 5.6 Agent listing (optional, Phase 2)

With frames arriving before YouTube, an agent can prepare listings while the host is still talking.
**The agent never creates a product.** It creates a **listing proposal**, a record only staff can see
in hg-studio. A product exists only after a CS rep presses Publish (D10).

#### Flow

1. **Detected.** hg-server sends sampled frames that changed (frames that barely differ are
   skipped) to a small, cheap vision model, which answers "is a new product being shown, and
   where?". On a yes, hg-server creates a proposal in `detected` with that frame's stream timestamp.
   hg-studio shows a placeholder card ("Product spotted at 00:42:10…").
2. **Drafting.** hg-server collects 3–4 sharp frames around that moment from the ring buffer, the
   last minute of transcript, and similar existing products (image matching). A stronger model fills
   in the proposal's fields.
3. **Ready.** The filled-in card appears in hg-studio's quick-listing panel. Fields the agent was
   unsure of are highlighted, and empty fields must be filled in by the rep.
4. **Published or dismissed.** The rep corrects what is wrong and presses **Publish**. That runs
   the normal quick-listing transaction (5.2) with the proposal's ID, and the proposal records the
   item it became. **Dismiss** records a reason (`not_a_product`, `duplicate`, `wrong_item`, `other`).

#### States

```
detected ──▶ drafting ──▶ ready ──┬──▶ published   (CS rep pressed Publish)
    │            │                ├──▶ dismissed   (CS rep dismissed, with a reason)
    │            └──▶ failed      └──▶ expired     (no action within the time limit)
    └──────────────▶ merged        (same product as an open proposal or a live item)
```

- `failed`: the model call failed or returned an unusable answer. The card offers **Retry** and
  **List manually**, which opens the normal quick-listing form with the frames already attached.
- `expired`: not acted on within 10 minutes (a setting) or by the end of the stream. The record is
  kept for measurement.
- `merged`: see "No duplicates" below.
- A proposal moves to `published` only inside the quick-listing transaction, so it can be published
  once at most. A second Publish returns the item already created.

#### What a proposal holds

```json
{
  "id": "prop_01J…",
  "show_id": "show_01J…",
  "status": "ready",
  "detected_at_ms": 2530400,
  "evidence": {
    "frames": ["cap_…a", "cap_…b", "cap_…c"],
    "best_frame": "cap_…b",
    "crop": { "x": 412, "y": 160, "w": 520, "h": 610 },
    "transcript": { "from_ms": 2471000, "to_ms": 2531000, "text": "…이 가방 십사만구천원…" }
  },
  "kind": "new",
  "match": null,
  "fields": {
    "title":   { "value": "Woven shoulder bag", "confidence": "high" },
    "price":   { "value": 149000, "heard": "십사만구천원", "confidence": "medium" },
    "options": [
      { "name": "Color", "values": [ { "label": "Brown", "hex1": "#4a3f35" } ], "confidence": "high" },
      { "name": "Size",  "values": [], "confidence": "none" }
    ],
    "stock":   { "mode": "until_sold_out", "confidence": "low" }
  },
  "models": { "watcher": "…", "drafter": "…", "cost_won": 92 },
  "published_item_id": null,
  "dismiss_reason": null,
  "merged_into": null
}
```

| Field | Meaning |
|---|---|
| `evidence` | Why the agent proposed what it did: full-resolution frames from the relay buffer (stored as captures), the crop for the product photo, and the transcript excerpt with its stream times. |
| `kind` | `new` (create a product), `existing_product` (a catalog product matched; Publish adds it to the stream without creating a product, and the price can still be changed), or `already_in_show` (this stream already has it as an item). |
| `match` | For `existing_product` / `already_in_show`: the product and show item, with the match score. |
| `fields.*.confidence` | `high`, `medium`, `low` or `none`. hg-studio highlights `low` and `medium`; `none` leaves the field empty and the rep must fill it in. |
| `fields.price.heard` | The words the price came from, so the rep can check it at a glance. The price is the field most likely to be costly if wrong. |
| `models` | Which models drafted it and what it cost, so cost per listing can be tracked. |

#### No duplicates

The host often shows the same bag several times, or comes back to it later. Before creating a
proposal, hg-server compares the new detection with:

- **open proposals in this show**: the same product within a few minutes is merged into the
  existing proposal (its frames and transcript are added). The new detection is stored as `merged`
  with `merged_into`.
- **live items in this show**: a match becomes `already_in_show`. The card says "No. 9 is being
  shown again" and offers **Pin as worn or held today** instead of Publish.

#### Approval rules (D10)

- Only staff with the `cs` role or above can publish or dismiss a proposal.
- The agent cannot call Publish. There is no path from a proposal to a live product that skips a
  person.
- Publishing records both `published_by` (the rep) and the proposal ID on the item, so every listing
  can be traced back to its evidence.

#### Studio updates in real time

hg-studio's staff SSE stream (`/studio-api/shows/{id}/events`) carries `proposal.detected`,
`proposal.ready`, `proposal.failed`, `proposal.merged`, `proposal.published`, `proposal.dismissed`
and `proposal.expired`. Each event carries the full proposal. When two reps watch the console, a
proposal one of them publishes disappears from the other's queue at once.

#### Measuring accuracy

Every proposal is kept with its outcome, so hg-studio can report per stream:

- the share of proposals published versus dismissed (and why);
- per field, how often the rep changed the agent's value. Price matters most;
- time from detection to `ready`, and from `ready` to published;
- model cost per published item.

This is the evidence for any later decision to let the agent publish some items alone (for
example, matched existing products with a high price confidence).

#### Cost and timing

- Estimated model cost: **about $5–10 per 3-hour stream** with about 40 listings (cheap watcher
  plus a stronger model per listing), before speech-to-text. To be confirmed with a pilot on a
  recorded stream.
- A proposal can be ready before YouTube viewers even see the product. Publishing at that moment is
  fine (the item appears as the host starts talking about it), but the console shows the YouTube
  delay so the CS rep knows what viewers currently see.

#### Where it runs: hg-agent

The agent's load is small (one frame every 2 s to the watcher, 40–60 drafting calls per stream, one
audio stream, about 70 MB of frames in memory). It still runs as its **own process**, for reasons
other than scale:

- **Failure isolation.** It handles large images, slow model calls, retries and a third-party
  speech service. A memory spike or stuck loop there must not slow checkout at the moment the host
  holds up an item and hundreds of viewers press Buy.
- **It changes more often.** Prompts, models, thresholds and duplicate rules will be tuned stream by
  stream. Restarting the agent never restarts the shop.
- **It is optional.** If the agent is off, broken or over budget, manual listing works as before.
- **Room for another runtime.** Self-hosted image matching or speech models would bring Python or
  a GPU, which do not belong in the shop's process.

**How:** the same Go codebase and image, started as `hg-server --agent` in its own container. It is
worth a separate program only if models move to self-hosting (Python, GPU), several streams run at
once, or another team owns it.

**The split:** hg-agent perceives, hg-server decides.

| Stays in **hg-server** | Runs in **hg-agent** |
|---|---|
| Frame ring buffer (manual capture needs it in Phase 1) | Sampling frames and skipping near-duplicates |
| `listing_proposals` table and its states | Watcher and drafting model calls, retries, cost tracking |
| Duplicate checks against open proposals and live items | Speech-to-text from the relay's audio, transcript windows |
| Publish (quick-listing transaction), dismiss, expiry | Image embeddings for matching |
| Nearest-product search over stored embeddings (pgvector) | Model, prompt and threshold configuration |
| Studio SSE events, permissions, accuracy reports | Per-stream cost cap |

**Internal API** (`/internal-api/…`, private network, service token; hg-server validates every
request as it would a user's):

| Method | Path | Does |
|---|---|---|
| GET | `/events` | SSE: shows started and ended, items published (to embed their photos) |
| GET | `/shows/{id}/frames?from=&scale=` | Sampled frames from the ring buffer, downscaled for the watcher |
| POST | `/shows/{id}/captures` `{ at }` | Keep a full-resolution frame as a capture for evidence |
| POST | `/shows/{id}/detections` | Report a sighting. hg-server answers `detected` (with a proposal ID), `merged` or `already_in_show`, so the agent drafts only new ones |
| PUT | `/proposals/{id}/draft` | Submit drafted fields, evidence and model cost; the proposal becomes `ready` |
| POST | `/proposals/{id}/fail` | Drafting failed, with a reason |
| POST | `/catalog/similar` `{ embedding }` | Nearest existing products and live items |
| PUT | `/products/{id}/embedding` | Store a photo's embedding after publishing |
| POST | `/agent/heartbeat` | Agent status, model latency, cost so far this stream |

**When the agent is down or over budget:**

- hg-server marks a proposal still `drafting` after 2 minutes as `failed`. The card offers
  **List manually** with its frames.
- hg-studio shows "Agent offline" or "Agent paused: cost cap reached" from the heartbeat. Manual
  capture and quick listing are unaffected.
- The agent holds no state that matters. After a restart it reads the open proposals and continues
  from the current position in the stream.

#### Data and API

- Table `listing_proposals`: `id`, `show_id`, `status`, `detected_at_ms`, `evidence` (JSONB, capture
  IDs), `kind`, `match` (JSONB), `fields` (JSONB), `models` (JSONB), `published_item_id`,
  `dismiss_reason`, `merged_into`, `decided_by`, `created_at`, `decided_at`.
- `GET /studio-api/shows/{id}/proposals?status=ready` lists the queue.
- `POST /studio-api/shows/{id}/items/quick` takes an optional `proposal_id`; the request carries
  the fields as the rep corrected them.
- `POST /studio-api/proposals/{id}/dismiss` `{ reason }` and `POST /studio-api/proposals/{id}/retry`.

### 5.7 After the stream

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
- **Order number** `HG-YYMMDD-XXXXX` (e.g. `HG-261107-7KQ4M`): the order date in Korea time and five
  random characters (digits 2–9 and consonants, so nothing to confuse and no words), which do not
  reveal how many orders a day brings. Easy to read out on the phone or in KakaoTalk; the internal
  ID stays a UUID.
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

- **account**: customers (Kakao, phone or email sign-in, 7.1), staff (roles `owner`, `manager`, `cs`), saved
  addresses with an optional customs number. Sessions are HttpOnly cookies held by hg-server, not
  tokens in the browser. Customers sign in on storefront, staff on hg-studio, with separate cookies
  and separate API surfaces (3.3).
- **Roles** decide Studio access: `cs` can run the live console, list items, answer chat and move
  orders through inspection; `manager` adds refunds, stream setup and settings; `owner` adds staff
  management.
- **support**: one conversation per customer, messages with optional product or order references,
  SSE to both sides. Replaces KakaoTalk for 1:1 questions. The Studio Chat tab is its inbox.
- **notify**: Telegram alerts for staff (new order, paid, listing failed, stream started), email to
  customers (order paid, shipped, chat reply). Kakao messages are Phase 3.

### 7.1 Customer sign-in: email now, Kakao and phone later (D3)

**Now:** email and password only (built: `/api/auth/register`, `/api/auth/login`). Sending email
(SMTP) is deferred too (2026-11-07), so there is no password reset or email verification yet;
until then a customer who forgets their password contacts staff. **Later**
(on hold, 2026-11-07): Kakao and phone, designed below so adding them does not change existing
accounts.

One customer can have several **identities**; any of them signs in to the same account.

| Method | How | Needs |
|---|---|---|
| **Kakao** | Kakao Login (OAuth). hg-server receives the Kakao user id and stores it as an identity. | A Kakao Developers app. Basic login works with a nickname only; reading the phone number or email from Kakao needs a verified business app, so that part may wait for the permit. |
| **Phone** | Enter a Korean mobile number, receive a 6-digit code by SMS, enter it. | An SMS provider with a registered sender number, which needs business registration, so this may also wait for the permit. Until then a fake SMS provider logs the code (development only). |
| **Email** | Email and password (built). | Nothing. Email verification can come later. |

Rules:

- **Table `customer_identities`** (`customer_id`, `provider` = kakao / phone / email, `subject`
  = Kakao id, phone number in E.164 or email, `verified_at`), unique on (`provider`, `subject`).
  Today's `customers.email` and password move into it.
- **No automatic merging.** Signing in with Kakao never joins an existing account because an email or
  phone happens to match; a signed-in customer adds another method from their account page (the new
  identity is verified there first). This prevents someone from taking over an account through an
  unverified email.
- **The verified phone number** becomes the customer's contact number and is suggested at
  checkout.
- **Phone codes:** 6 digits, valid 3 minutes, 5 tries, only a hash stored; at most 5 codes per
  number per hour and per IP. Table `phone_codes`.
- **Provider interfaces** like payment: `KakaoAuth` and `SMSSender`, each with a fake for
  development and tests, so the flows can be built and tested before the permit.

### 7.2 Hosting on H-Global's server (D7)

```
Internet ── TLS proxy (Caddy: automatic HTTPS) ──┬── storefront   hglobal domain
                                                 ├── hg-studio    studio. subdomain
                                                 └── hg-server    /hooks only
OBS ── SRT/RTMP ──▶ media-relay ──▶ YouTube
hg-server ──▶ PostgreSQL 18, files on local disk (/data)
nightly: pg_dump + files ──▶ off-machine backup
```

What we need to know about the server before the first deployment:

| Item | Why |
|---|---|
| OS, CPU, RAM, free disk | Sizing; PostgreSQL 18 and the photo and video files live on it |
| Public IPv4, open ports (80, 443, and the SRT/RTMP port) | TLS certificates and the broadcaster's upload |
| Upload bandwidth | The relay sends the stream to YouTube: about 6 Mbps per stream, ~8 GB for 3 hours |
| Domain name | Storefront, studio subdomain, ingest address |
| Docker available, who has admin access | Deployment by Docker Compose |
| Where backups can go (another machine or cloud storage) | Every copy of the data must not live on one disk |

## 8. API outline

storefront calls `/api/…`, hg-studio calls `/studio-api/…` (staff session and role required). Paths
below are relative to those prefixes. The full contract lives in `api/openapi.yaml`.

| Area | storefront → `/api` | hg-studio → `/studio-api` |
|---|---|---|
| Live | `GET /shows/current`, `GET /shows/{id}`, `GET /shows/{id}/events` (SSE), `GET /shows/{id}/recap`, `GET /shows?status=ended` | `POST/PATCH /shows`, `POST /shows/{id}/start`, `/end`, `POST/DELETE /shows/{id}/items`, `PUT /shows/{id}/items/order`, `GET /shows/{id}/ingest` (URL + key), `GET /shows/{id}/preview` (WebRTC session), `GET /shows/{id}/frames?from=` (ring buffer), `POST /shows/{id}/captures`, `POST /shows/{id}/items/quick` (optional `proposal_id`), `GET /shows/{id}/proposals`, `POST /proposals/{id}/dismiss`, `/retry` (Phase 2), `POST /items/{id}/pin`, `/unpin`, `/sold-out`, `GET /shows/{id}/console` |
| Catalog | `GET /products`, `GET /products/{id}` (incl. broadcasts and inspection media) | `PATCH /products/{id}`, options, variants, stock, media upload, `GET /catalog/suggestions`, `GET /catalog/incomplete` |
| Checkout | `GET/PUT /cart`, `POST /checkout` (Buy now / from cart, takes holds), `POST /checkout/{id}/pay` | — |
| Orders | `GET /orders`, `GET /orders/{id}`, `POST /orders/{id}/cancel` | `GET /orders`, `POST /orders/{id}/inspection`, `/customs-check`, `/ship`, `/deliver`, `/refund`, `GET /orders/events` (SSE) |
| Payment | — (provider webhooks arrive at `/hooks/payments/{provider}`) | `GET /payments`, refunds via orders |
| Account | `POST /auth/kakao`, `/auth/email`, `GET/PUT /me`, addresses | `POST /auth/login`, staff management |
| Support | `GET/POST /support/messages`, `GET /support/events` (SSE) | `GET /support/conversations`, reply, close, `GET /support/events` (SSE) |

hg-agent calls `/internal-api/…` (service token, private network only); its endpoints are listed in
5.6, "Where it runs: hg-agent".

## 9. Data (one PostgreSQL database)

| Module | Tables |
|---|---|
| account | `customers`, `staff` (sessions live in platform; `addresses` not built yet) |
| catalog | `products`, `product_options`, `option_values`, `variants`, `product_media` |
| live | `shows`, `show_items`, `captures`, `listing_proposals` (Phase 2) |
| checkout | `checkouts`, `checkout_lines`; holds are `held_qty` on `variants` (`carts` not built yet) |
| order | `orders` (incl. carrier and tracking), `order_lines` (snapshot of title, options, price, photo), `order_media`, `order_history` |
| payment | `payments`, `refunds`, `webhook_receipts` |
| support | `conversations`, `messages` |
| platform | `events` (event log + SSE replay), `sessions`, `schema_migrations`; idempotency is `show_items.idempotency_key` and `webhook_receipts` |
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

| Phase | hg-server | storefront | hg-studio |
|---|---|---|---|
| **0 — Decide** (with the 11/08 design review) | Answer D1–D10. Agree this plan and `api/openapi.yaml` v0. | Design draft approved | Design draft approved |
| **1 — MVP** (about 6–8 weeks; the apps are built alongside hg-server against the contract and a mock server) | Platform (config, DB, migrations, sessions, SSE hub, jobs, storage). Accounts and staff roles. Catalog with spontaneous products, options, variants, stock, media. Live: shows, preselected No. 1–8, **media relay to YouTube with frame tap and preview**, capture from the ring buffer, quick listing, pin, sold out, SSE feed, replay offsets, auto-hide. Checkout with holds; orders through shipping; one payment provider; Telegram alerts; email for paid and shipped. | Home (live banner, next stream), live page with SSE, product sheet and page, cart, checkout with hold timer and customs number, sign-in, order history. | Sign-in, stream setup (ingest URL and key), live console (relay preview, capture with step-back, quick listing, published list, pin, sold out, YouTube push status), order handling (inspection, customs check, ship, refund), product editing. |
| **2 — Match and follow up** (about 3–4 weeks) | **Agent listing proposals (5.6)**: the `--agent` mode, `/internal-api`, speech-to-text; image matching (pgvector); recap; completion list; console stats per show; 1:1 chat; inspection media on the product page. | Recap page, chat, inspection section on the product page. | Agent proposals and match suggestions in quick listing, after-stream screen, chat inbox, console stats. |
| **3 — Later** | Kakao messages, stream reminders, YouTube chat, customs number lookup, reviews, analytics per item. | Reminders, reviews. | Analytics. |

Suggested build order inside Phase 1. Each step ends with something working across the programs
involved:

1. **Skeleton:** the three programs start under `docker compose`; storefront and hg-studio proxy to
   hg-server; `api/openapi.yaml` v0 and generated clients; migrations; staff sign-in in hg-studio.
2. **Catalog:** products, options, variants, stock and media in hg-server; product editing in
   hg-studio; product page in storefront.
3. **Broadcast relay:** media-relay container, ingest keys, copy to YouTube, frame ring buffer and
   preview, with a test stream from OBS; the YouTube push status in hg-studio.
4. **Live listing:** shows, preselected items, captures and quick listing in hg-server; stream setup
   and the live console in hg-studio.
5. **Live feed:** events, SSE hub, in-memory show snapshot and stock push in hg-server; the live page
   in storefront updates without reload.
6. **Buying:** checkout and holds, orders, payment adapter and webhook, hold and unpaid-order jobs in
   hg-server; cart and checkout in storefront; order handling in hg-studio.
7. **Finish:** YouTube viewer count, auto-hide job, Telegram and email; one end-to-end test that runs a
   stream in hg-studio, buys the item in storefront and ships it.

## 12. Design follow-ups

- **Checkout** (D1 decided): Korean address with postcode search (the free Kakao/Daum postcode
  widget in storefront), a Korean mobile number, and the customs number field. Replace the US
  address in Figma. The 10-minute hold timer already matches D4.
- **Sign-in** (D3): email and password for now. Leave room on the screen for Kakao and phone later.
- **Quick listing**: the Color and Size chips should come from recently used labels with a "+ Add"
  for new ones (4.2). Hex is optional, and two-tone uses two hex values on one option value, which
  matches the components board.
- **Stock**: the "Unknown — sell until marked sold out" toggle matches stock mode `until_sold_out`.
