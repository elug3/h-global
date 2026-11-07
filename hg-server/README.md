# hg-server

H-Global's backend: one Go program (a monolith) with one PostgreSQL 18 database and one file
store. It serves the two apps and runs the background jobs.

| Path | Used by | Session |
|---|---|---|
| `/api/…` | **storefront** (customer site) | cookie `hg_session` |
| `/studio-api/…` | **hg-studio** (H-Global Studio, staff) | cookie `hg_staff_session` + role |
| `/hooks/…` | payment provider | webhook signature |
| `/media/…` | anyone | public product photos |
| `/healthz` | load balancer | none |

The full HTTP contract is [`../api/openapi.yaml`](../api/openapi.yaml). The design and its
decisions are in [`../docs/plan/backend-plan.md`](../docs/plan/backend-plan.md).

## What is built

| Area | Status |
|---|---|
| Staff accounts and roles (`cs` < `manager` < `owner`), customer accounts, sessions | Built |
| Products created on the spot: photo + price required, free-form options with swatch colors, every option combination becomes a variant | Built |
| Stock: counted or "until sold out", holds that can never oversell, recently used option values | Built |
| Streams (shows): preselected No. 1–8, numbers locked at start, one live show at a time | Built |
| Capture upload and **quick listing in one transaction** (product, options, stock, photo, numbered item, viewer event) with idempotency | Built |
| Pin "worn or held today", sold out, keep listed, auto-hide N days after the stream | Built |
| Live feed to viewers (Server-Sent Events) with replay from `last_event_id`; stock changes pushed | Built |
| Product page "broadcast on" links, stream recap | Built |
| Checkout: Buy now holds stock for 10 minutes, address + customs code (PCCC), card or virtual account | Built |
| Orders: pending_payment → paid → inspecting → customs_check → shipped → delivered → completed; cancel with refund | Built |
| Inspection photos and videos per order | Built |
| Payment provider interface, webhook verification and idempotency, late-payment auto refund | Built, with a **fake provider** until one is chosen (plan D2) |
| Staff order feed (SSE) | Built |
| **Broadcast relay** (prototype): SRT ingest with a per-show passphrase, unchanged copy to YouTube, 2 fps frame buffer (120 s), MJPEG preview, capture from a buffered frame | Built ([docs/prototype/relay.md](../docs/prototype/relay.md)) |
| YouTube Data API, hg-agent, 1:1 chat, Telegram/email, Kakao/SMS login, password reset, cart, reviews | **Not built yet** (see "Next") |

## Run it locally

Requirements: Go 1.24+, PostgreSQL 18 (or Docker), and ffmpeg for the broadcast relay and its tests.

```bash
# PostgreSQL 18 in Docker, with a hglobal_test database for the tests
docker run -d --name hg-postgres -p 5432:5432 \
  -e POSTGRES_USER=hglobal -e POSTGRES_PASSWORD=hglobal_dev -e POSTGRES_DB=hglobal postgres:18
docker exec hg-postgres psql -U hglobal -c 'CREATE DATABASE hglobal_test'

# hg-server
cd hg-server
export HG_DATABASE_URL="postgres://hglobal:hglobal_dev@localhost:5432/hglobal?sslmode=disable"
export HG_COOKIE_SECURE=false HG_DEV_ROUTES=true
export HG_OWNER_EMAIL=owner@hglobal.test HG_OWNER_PASSWORD=owner-password
go run ./cmd/hg-server
```

Or everything in Docker: `docker compose -f deploy/compose.dev.yaml up --build` from the repository
root.

Migrations run on startup. The first owner is created from `HG_OWNER_EMAIL` / `HG_OWNER_PASSWORD`
when there are no staff yet.

```bash
# Sign in to H-Global Studio and create a stream
curl -c jar -H 'Content-Type: application/json' \
  -d '{"email":"owner@hglobal.test","password":"owner-password"}' localhost:8080/studio-api/auth/login
curl -b jar -H 'Content-Type: application/json' \
  -d '{"title":"Seoul warehouse tour","youtube_url":"https://youtu.be/VIDEO_ID"}' localhost:8080/studio-api/shows
```

### Process modes

```bash
hg-server              # HTTP and jobs in one process (default)
hg-server -jobs=false  # HTTP only
hg-server -http=false  # jobs only
hg-server -migrate     # apply migrations and exit
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `HG_DATABASE_URL` | (required) | PostgreSQL 18 URL. Pool size 20 unless the URL sets `pool_max_conns`. |
| `HG_HTTP_ADDR` | `:8080` | Listen address. |
| `HG_STORAGE_DIR` | `./data/storage` | Where files are stored (photos, frames, inspection files). |
| `HG_MEDIA_BASE_URL` | `/media` | Prefix of public photo URLs (set a CDN URL in production). |
| `HG_COOKIE_SECURE` | `true` | `Secure` flag on session cookies. `false` only for local HTTP. |
| `HG_ALLOWED_ORIGINS` | (none) | Comma-separated origins allowed to send state-changing requests with an `Origin` header (the two apps' public URLs). |
| `HG_OWNER_EMAIL`, `HG_OWNER_PASSWORD` | (none) | Creates the first owner when there are no staff. |
| `HG_HOLD_TTL` | `10m` | How long Buy now holds stock. |
| `HG_VIRTUAL_ACCOUNT_TTL` | `24h` | Deposit deadline for virtual-account orders (stock stays held). |
| `HG_STREAM_DELAY` | `10s` | Subtracted from capture times when computing replay offsets. |
| `HG_SHIPPING_FEE_WON` | `0` | Flat shipping fee per order. |
| `HG_STAFF_SESSION_TTL` | `12h` | Staff session lifetime. |
| `HG_CUSTOMER_SESSION_TTL` | `720h` | Customer session lifetime (30 days). |
| `HG_BCRYPT_COST` | `12` | Password hashing cost. |
| `HG_PAYMENT_PROVIDER` | `fake` | Only `fake` exists until the provider is chosen. |
| `HG_FAKE_PAYMENT_SECRET` | `dev-only-secret` | Signs fake webhooks. |
| `HG_DEV_ROUTES` | `false` | Enables `/api/dev/payments/{id}/approve` and `/fail`. Never in production. |
| `HG_RELAY_ENABLED` | `false` | Runs the broadcast relay for the live show (needs `ffmpeg`). |
| `HG_RELAY_SECRET` | (none) | Derives per-show SRT passphrases. At least 16 characters; required with the relay. |
| `HG_RELAY_LISTEN` | `0.0.0.0:9000` | SRT listen address (UDP). |
| `HG_RELAY_PUBLIC_HOST` | `localhost:9000` | Host and port shown to the broadcaster. |
| `HG_RELAY_SRT_LATENCY_MS` | `300` | SRT buffer for lost packets; raise it for poor uplinks. |
| `HG_RELAY_LOCAL_COPY` | `127.0.0.1:19400` | Local UDP port between ingest and the YouTube pusher. |
| `HG_YOUTUBE_RTMP_URL` | `rtmps://a.rtmp.youtube.com/live2` | YouTube ingest. |
| `HG_YOUTUBE_STREAM_KEY` | (none) | YouTube stream key; empty means frames only, no push. |
| `HG_FFMPEG` | `ffmpeg` | ffmpeg binary. |

## Code layout

```
cmd/hg-server/       main: config, database, migrations, HTTP server, jobs
migrations/          SQL migrations (embedded), applied in file-name order
internal/
  app/               wires every module into one handler and one job list; end-to-end tests
  platform/          plumbing without business rules
    apperr/          classified errors (invalid, not found, conflict, ...) with stable codes
    auth/            sessions (hashed tokens in cookies), passwords, RequireStaff/RequireCustomer
    config/          environment variables
    db/              pool, unit of work (InTx), after-commit hooks, migrations
    dbtest/          a fresh migrated schema per test
    events/          event log, in-process hub, Server-Sent Events with replay
    httpx/           JSON, error mapping, router that records routes, middleware
    ids/             UUIDv7
    imageutil/       image checks, crop, JPEG re-encode (strips metadata)
    jobs/            periodic jobs with advisory locks
    storage/         file storage interface + local filesystem
    upload/          multipart uploads, images and videos
  account/           staff and customers
  catalog/           products, options, variants, stock and holds, media, suggestions
  live/              shows, numbered items, captures, quick listing, viewer feed, auto-hide
  checkout/          Buy now holds, address, pay
  order/             order lifecycle, inspection, customs check, shipping, cancel/refund
  payment/           provider interface, fake provider, webhooks, refunds
  relay/             broadcast relay: ffmpeg ingest and YouTube push, frame buffer, Studio routes
```

Each module has `model.go` (types and rules), `service.go` (use cases), `postgres.go` (its SQL),
`http.go` (handlers) and tests.

### Rules

1. **A module owns its tables.** Other modules call its service, never its SQL.
2. **Business rules live in `model.go` and `service.go`.** Handlers decode, call the service and
   encode. `postgres.go` only reads and writes.
3. **Cross-module work shares one transaction.** `db.InTx` puts the transaction in the context and
   every store asks `db.Q(ctx)` for its querier. A nested `InTx` joins the outer one. Quick listing
   and checkout use this to commit everything together.
4. **Changes are announced as events.** `events.Bus.Publish` writes the event row in the same
   transaction; after commit it reaches the SSE hub. `db.AfterCommit` runs other after-commit work
   (deleting replaced files, provider calls).
5. **Imports go one way.** `catalog` ← `live`, `order` ← `checkout`; `order` → `payment`.
   `payment` tells `order` about money through the small `payment.OrderHook` interface, so there is
   no import cycle.
6. **Network calls stay outside transactions.** The payment provider is called after commit
   (`payment.Begin`) or while holding only a row lock on the payment (refunds).

## How the main flows work

### Quick listing (`POST /studio-api/shows/{id}/items/quick`)

1. Studio uploads a frame: `POST /studio-api/shows/{id}/captures` stores it unchanged and records
   its offset in the stream.
2. Quick listing crops the frame, re-encodes it as JPEG and stores it as a public photo. This is
   **before** the transaction, because it is slow.
3. One transaction locks the show row (listings in one show are serialized), takes `next_number`,
   creates the product with options, variants, stock and photo, inserts the numbered item and
   publishes `live.item_published`.
4. If anything fails, everything rolls back, the photo is deleted, and the number is not used.
   The same `Idempotency-Key` header returns the item already created.

### Holding stock

`catalog.Hold` is one conditional `UPDATE … WHERE stock_qty − held_qty − sold_qty >= $n`. When no
row changes, the answer is `409 sold_out`. A test races 20 buyers for one unit; exactly one wins.
`available_qty` is a PostgreSQL 18 **virtual generated column**, computed when read.

| Event | Stock change |
|---|---|
| Buy now (`POST /api/checkout`) | held + n |
| Checkout expires or is abandoned | held − n |
| Order paid (webhook) | held − n, sold + n |
| Unpaid order canceled | held − n |
| Paid order canceled before shipping | sold − n (stock returns) |
| Canceled after shipping | none |

### Live feed

A storefront page reads `GET /api/shows/{id}` (which carries `last_event_id`), then opens
`GET /api/shows/{id}/events?last_event_id=…`. Events after that id are replayed from the `events`
table, then live events follow. Every catalog change to a featured product (price, title, photos,
stock, sold out) is pushed as `live.item_updated` with the full item. Event ids are global; events
are kept for 24 hours; `event: reset` tells the client to reload the snapshot. The stream sends a
comment every 15 s to keep proxies open; proxies must not buffer `text/event-stream`.

One process holds every stream. If hg-server ever runs as several HTTP processes, the hub needs a
PostgreSQL `LISTEN/NOTIFY` fan-out (plan 5.3).

### Orders and payment

1. `POST /api/checkout` holds stock and opens a checkout (expires after `HG_HOLD_TTL`).
2. `PUT /api/checkout/{id}/shipping` sets the address and the customs code (`P` + 12 digits).
3. `POST /api/checkout/{id}/pay` creates the pending order (the holds move with it) and a pending
   payment in one transaction, then asks the provider to start it. Card: send the customer to
   `payment.redirect_url`. Virtual account: show `payment.virtual_account`.
4. The provider's webhook (`POST /hooks/payments/{provider}`) is signature-checked and recorded in
   `webhook_receipts`, so repeats do nothing. On success the order becomes `paid` and the holds
   become sold, in one transaction.
5. A payment that arrives after the order was canceled is refunded automatically.
6. Staff: inspection files (`POST …/inspection`, the first moves the order to `inspecting`) →
   `…/inspection/complete` → `…/customs-check` (requires a valid PCCC) → `…/ship` → `…/deliver`.
   The customer confirms receipt (`POST /api/orders/{id}/receipt`).
7. Cancel: the provider refunds first, then the order changes.

With the fake provider, `POST /api/dev/payments/{id}/approve` plays the customer paying
(`HG_DEV_ROUTES=true`).

### Jobs

| Job | Every | Does |
|---|---|---|
| `checkout-expiry` | 30 s | Releases holds of checkouts past `expires_at` |
| `unpaid-orders` | 1 min | Cancels orders past `payment_deadline` |
| `auto-hide` | 1 min | Hides products whose stream items passed `hide_at`, unless another show still features them |
| `prune` | 1 h | Deletes events older than 24 h and expired sessions |

Jobs run one after another from one scheduler goroutine, each under a PostgreSQL advisory lock, so
several processes never run the same job at once. Every job is safe to repeat.

## Tests

Most tests run against a real PostgreSQL 18. Each test gets its own schema, migrated and dropped
afterwards, so tests can run in parallel. Without `HG_TEST_DATABASE_URL`, database tests are
skipped.

```bash
export HG_TEST_DATABASE_URL="postgres://hglobal:hglobal_dev@localhost:5432/hglobal_test?sslmode=disable"
go test ./...
go vet ./...
```

| Test | Covers |
|---|---|
| `internal/app` `TestLiveStreamToDelivery` | The plan's end-to-end flow over HTTP: go live, viewer stream, capture, quick listing, two buyers racing for one unit, pay, inspect, customs check, ship, deliver, receipt, replay links, recap |
| `internal/app` `TestPaidCancelRefundsAndReturnsStock`, `TestAccessRules` | Refund and stock return; roles, session separation, cross-site origin check |
| `internal/app` `TestRoutesMatchOpenAPI` | Every route is in `api/openapi.yaml` and nothing extra is documented |
| `internal/app` `TestJobsRunOnASmallPool` | Jobs cannot deadlock the connection pool |
| `internal/catalog` | Variant combinations, validation, concurrent holds never oversell, stock bookkeeping, suggestions |
| `internal/live` | YouTube links, numbering and reordering, quick listing rollback and idempotency, events, auto-hide |
| `internal/checkout` | Hold expiry, unpaid-order expiry and late-payment refund, webhook signature and idempotency, virtual accounts |
| `internal/platform/db` | PostgreSQL 18 (`uuidv7()`, virtual generated column), transactions and after-commit hooks |
| `internal/relay` | Frame splitting, buffer window, ingest keys; with ffmpeg: **latency measurement** and wrong-passphrase rejection |
| `internal/app` `TestRelayToQuickListing` | Live show → relay listens → SRT broadcast → frames, preview → capture from buffer → quick listing with replay offset → show ends, relay stops |

## Operations

- **Docker image:** `hg-server/Dockerfile` builds a static binary on a distroless base;
  `deploy/compose.dev.yaml` runs it with PostgreSQL 18.
- **Health:** `GET /healthz` pings the database (503 when it cannot).
- **Relay:** the image includes ffmpeg; open UDP 9000 for SRT. The relay runs in the HTTP process
  (its frame buffer is served there).
- **Logs:** JSON on stdout, one line per request.
- **Backups:** the database and `HG_STORAGE_DIR` (plan section 10).
- **Behind a proxy:** the apps proxy `/api` and `/studio-api` on their own origins (plan 3.3); set
  `HG_ALLOWED_ORIGINS` to their public URLs and keep `HG_COOKIE_SECURE=true`.

## Next

In the order of the plan's build steps:

1. **Measure the relay on H-Global's server** with a real camera and YouTube
   ([docs/prototype/relay.md](../docs/prototype/relay.md)).
2. **YouTube Data API**: viewer count and actual start time.
3. **Notifications**: Telegram alerts for staff, email for paid and shipped.
4. **Payment provider** (plan D2): a real adapter behind `payment.Provider`, plus a reconcile job.
5. **Customer sign-in with Kakao** (plan D3), saved addresses, cart.
6. **Phase 2:** hg-agent listing proposals, image matching, 1:1 chat, console stats.
