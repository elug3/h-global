# storefront

The H-Global customer website: watch the YouTube stream, buy what the host shows, follow the order
to the door. Korean, mobile first (the live page has a desktop layout too). Server-rendered, so
product and stream links shared on KakaoTalk show a title, price and photo.

It talks only to hg-server's `/api`, typed from [`../api/openapi.yaml`](../api/openapi.yaml).
Screens follow the Figma draft (boards 1–6 and 10–15).

## Screens

| Screen | Route | What it does |
|---|---|---|
| 홈 | `/` | During a stream: the LIVE banner and "보면서 쇼핑하기". Otherwise the next-stream card, **only** if staff turned on "방송 예정으로 공개" in Studio (board 15); then past streams and products still on sale. |
| 라이브 페이지 / 다시 보기 | `/shows/:id` | YouTube player on top (left on desktop), "오늘 착용/소지", every item with its No. badge. Items listed on air appear **without reloading** (Server-Sent Events) with a "No.N 방금 등록" notice; sold-out items stay in place. After the stream it is the recap: same numbers, "이 장면 보기" jumps the player to the moment, and the date sales end. |
| 상품 시트 | on the live page | Tapping a card opens a sheet over the stream: photo, price, colors (two-tone swatches), sizes, quantity, stock left, 바로 구매, the moment in the stream, 상세 정보. |
| 상품 페이지 | `/products/:id` | Photos, options, the streams it was shown in (replay link to the second), details and specs, inspection photos and video. Share tags (`og:*`) are in the server HTML. |
| 로그인 / 회원가입 | `/login`, `/signup` | Email (D3). Buy now while signed out goes to sign-up showing the product, then back to it with the same choice. Kakao is shown as "곧 지원". |
| 주문/결제 | `/checkout/:id` | The 10-minute hold countdown, items, address with Kakao (Daum) postcode search, 개인통관고유부호 checked as you type (P + 12 digits), card or virtual account. The address is remembered on the device for the next order. |
| 테스트 결제 | `/pay/:paymentId` | Stands in for the card page while hg-server runs the **fake** provider (D2). With a real provider, checkout sends the buyer to the provider's page instead. |
| 결제 완료 / 입금 안내 | `/orders/:id/done` | Card: order number and what happens next. Virtual account: bank, account (copy), amount, deadline. |
| 내 주문 | `/orders` | Orders with status, tabs (전체 / 진행 중 / 완료), "검수 완료" notice, 입금 계좌 보기 and 주문 취소 while unpaid, 수령 확인 when delivered. |
| 주문 상세 | `/orders/:id` | Progress (결제 → 검수 → 통관 확인 → 배송 → 도착), inspection photos/video once inspection is done, tracking number, customs code (editable until staff check it), address, amounts, cancel (until inspection starts), receipt. |

Not yet: cart (hg-server has none; Buy now covers live buying), reviews, wishlist, 1:1 chat,
stream reminders, category filters, saved addresses on the account (kept on the device for now).

## Run it

```bash
# 1. hg-server on :8080 (see ../hg-server/README.md), allowing this origin and the fake payments:
#    HG_ALLOWED_ORIGINS=http://localhost:3000 HG_DEV_ROUTES=true
# 2. storefront
cd storefront
npm install
npm run dev            # http://localhost:3000, forwards /api and /media to :8080
```

`HG_API_URL` is where hg-server is (default `http://localhost:8080`). The server-side loaders call
it directly with the visitor's cookie; the browser calls `/api` on the site's own origin.

| Script | Does |
|---|---|
| `npm run dev` | Development server (server rendering, hot reload, API proxy) |
| `npm run build` | Route types, type-check, build into `build/` |
| `npm start` | Run the built app (`react-router-serve`, port `PORT`, default 3000) |
| `npm run typecheck` | Route types and TypeScript only |
| `npm run gen:api` | Regenerate `app/api/schema.d.ts` from `../api/openapi.yaml` |
| `npm run test:e2e` | Browser test against a real hg-server (see below) |

## How it is built

- **React Router 8 in framework mode** (server rendering), **Vite 8**, **TypeScript 5.9**.
- **Data:** each route's `loader` runs on the server and calls hg-server with the visitor's cookie
  (`serverApi`), so signed-in pages render on the server too; 401 redirects to sign-in and back.
  Changes (sign-in, Buy now, paying, cancel) are made from the browser to `/api` on the same origin,
  so hg-server's HttpOnly session cookie is set and sent by the browser itself.
- **Live updates:** `EventSource` on `/api/shows/{id}/events` from the snapshot's `last_event_id`;
  every item event carries the full item, so nothing is refetched.
- **Text:** every Korean string is in `app/i18n/ko.ts`. Times are shown in Korea time.
- **Styles:** `app/styles.css`, tokens from the Figma components board (red is only for LIVE).

```
app/
  root.tsx, routes.ts      document, routes
  api/                     client.ts (browser and server clients), schema.d.ts (generated)
  i18n/ko.ts               all text
  lib/format.ts            ₩, dates in Korea time, customs code check
  components/              ui (icons, LIVE, No. badge, swatch, stepper), ItemCard, BuyPanel,
                           AddressForm (postcode search), FlowHeader
  routes/                  layout, home, show, product, login, signup, checkout, test-pay, done,
                           orders, order
e2e/                       Playwright test
```

## Test

`npm run test:e2e` starts hg-server (`go run`, fresh database, fake payments) and the dev server,
then Chromium on a phone-sized screen:

a stream is announced → home shows the next-stream card → it goes live, staff list No.1 on air
(capture → quick listing) → home shows the LIVE banner → live page → No.2 appears without reloading
with the notice → No.1's sheet: 2 left, a color must be chosen → Buy now while signed out → sign-up
shows the product → back with the same color → checkout holds it → address, a wrong customs code is
caught → test payment approved → order `HG-YYMMDD-XXXXX` paid → the ivory bag is sold out for
another viewer, black is not → staff inspect → the buyer sees the photo and can no longer cancel →
shipped, delivered → 수령 확인 → 구매 확정 → No.2 by virtual account with the saved address →
account shown → cancelled from 내 주문 → product HTML has `og:title` and `og:image` → desktop layout.

Needs Go, psql and PostgreSQL 18; the database in `HG_E2E_DATABASE_URL` (default
`postgres://hglobal:hglobal_dev@localhost:5432/hglobal_e2e`) is wiped at the start.
`HG_SCREENSHOT_DIR=/some/dir` saves screenshots.

## Production

`npm run build && npm start` (or the `Dockerfile`) behind the TLS proxy, with `/api`, `/media` and
`/hooks` going to hg-server on the same domain: see [`../deploy/Caddyfile.example`](../deploy/Caddyfile.example).
Set `HG_API_URL` to hg-server's internal address, and add the site's origin to hg-server's
`HG_ALLOWED_ORIGINS`.
