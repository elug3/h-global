# H-Global Live Commerce Plan (draft)

> H-Global: an e-commerce service that lists bags in real time during YouTube live streams.
> Status: draft for discussion. Feeds the **design draft due 2026-11-08** (see `docs/todo/h-global-todo.md`).

## 1. Core idea
YouTube hosts the video; H-Global hosts the shop. While a seller presents a bag on stream, an operator
"drops" that bag onto a live shop page synced to the stream. Viewers buy on H-Global, not in YouTube chat.

Why not sell inside YouTube? YouTube Shopping requires approved merchant integrations and gives little control
over one-of-a-kind items, inspection media and customs. A companion page keeps checkout, stock, and the
customs flow under our control, linked from the stream description, pinned chat message and on-screen QR code.

## 2. Users
| Role | Needs |
|---|---|
| Viewer / buyer | See the bag on stream, tap once, reserve before someone else does, pay, track customs/shipping |
| Host (on camera) | Know what is next and what sold, without touching a computer |
| Operator (off camera) | Queue products, push "now showing", set price/stock, close sales, answer chat |
| Admin | Register products (Dupli1-style form), inspection media, orders, customs |

## 3. Live flow
1. **Before stream**: register bags (photos / description / specs / color variants) → build a *show rundown* (ordered list).
   Schedule stream; page `h-global.../live/{showId}` goes up with a countdown and the rundown preview.
2. **During stream**: operator clicks **Now showing** → every viewer's page updates in < 1s (product card, price, Buy button).
   Buyer taps **Buy** → item is **reserved for N minutes** (one-of-a-kind stock) → checkout.
   Sold state is pushed to all viewers and to the host's monitor ("SOLD to @name").
3. **After stream**: unsold items become normal listings; the stream VOD gets product timestamps
   (chapters) so later viewers can jump to and buy each bag.
4. **Fulfilment** (from Dupli1 TODO): order → inspection photo/video → overseas customs number entry/verify → shipping.

## 4. Key design decisions
- **Stock = 1 for most bags.** Reservation hold (e.g. 5 min) with atomic decrement in DB; released on timeout. Waitlist "next in line" if the buyer drops.
- **Sync to stream**: we drive state from operator actions, not from video timing. YouTube live latency (2–30s) is
  unavoidable, so the operator can "pre-arm" a drop and the page shows "coming up".
- **Real-time channel**: WebSocket/SSE fan-out (or a managed service such as Pusher/Ably/Supabase Realtime) — read-heavy, write-rare.
- **YouTube integration (phase 2)**: YouTube Data API `liveChatMessages` to read chat (e.g. detect "buy 3" intents, auto-post pinned link per product). Respect API quota.
- **Embed**: YouTube iframe embed on our page so viewers watch + buy in one screen (mobile-first, video top / product sheet bottom).

## 5. Data model (initial)
- `Product` (from master catalog; hex color / two-tone swatch fields)
- `Variant` (size, color, stock)
- `Show` (youtubeVideoId, scheduledAt, status)
- `ShowItem` (showId, productId, order, state: queued → live → sold/unsold, startedAt → VOD chapter)
- `Reservation` (variantId, userId, expiresAt)
- `Order` (status: paid → inspecting → customs_pending → shipped → delivered; customs number)
- `InspectionMedia` (orderId, photos/video)

## 6. Screens for the design draft (due 11/8)
1. Live shop page – mobile (video + current product sheet + "up next" + sold ticker)
2. Live shop page – desktop
3. Product sheet / quick checkout with reservation countdown
4. Operator console (rundown, Now showing, mark sold, viewer/sales counters)
5. Host monitor view (big text: current item, price, SOLD alerts)
6. PDP after stream (color swatch, inspection media section, no Official Price)
7. Order tracking incl. customs number step

## 7. Phases
| Phase | Scope | Target |
|---|---|---|
| 0 Design | Screens above, flow diagrams | 2026-11-08 |
| 1 MVP | Product registration, Show/rundown, live page with embed, Now showing push, reservation + checkout, operator console | +4–6 weeks |
| 2 Ops | Inspection media upload, customs flow, 1:1 chat, VOD chapters, analytics (views → taps → sales per item) | +4 weeks |
| 3 Growth | YouTube chat integration, waitlist, reviews, notifications ("show starts in 10 min"), multi-host | later |

## 8. Open questions
- Payment provider & currencies (domestic vs overseas buyers)? Customs ⇒ which destination countries first?
- Reservation length and whether unpaid holds get penalties.
- Auction/bidding format or fixed price only?
- Who operates during the stream — dedicated operator, or the host from a tablet?
- Tech stack preference (e.g. Next.js + Postgres + managed realtime) and hosting.
- KPIs for the first live: conversion per item, time-to-sold, checkout drop-off.
