# H-Global Live Commerce Plan (draft v2)

> H-Global sells products (bags, cosmetics, and more) in real time during YouTube live streams.
> Status: draft for discussion. Feeds the **design draft due 2026-11-08** (see `docs/todo/h-global-todo.md`).
> v2 (2026-11-03): rebuilt around **capture-and-list from H-Global Studio**, based on the requirements below.

> **Naming:** the staff tool is **H-Global Studio** (code and folder name: `hglobal-studio`; formerly "manage-web").
> Always say "H-Global Studio" in full so it isn't confused with YouTube Studio, which the team also uses during streams.

## 1. Why we need this

- **Products we see for the first time on location.** Streams filmed while touring markets and warehouses
  show items that were never registered and have no product data. Sometimes we only learn on air whether
  an item is a toner or a mist, so it cannot be registered in advance.
- **Viewers must be able to buy right away.** "What I wanted was for the items currently being promoted to go live immediately."
- **Sales keep going after the stream.** Products stay up for a few days, and one more ad goes out to
  regular KakaoTalk customers after the stream.
- **No commission.** Platforms like ClickMate already offer this. Building it on our own site avoids their fees.

If a stream only sells products that are already registered, all we need is the order in which they are featured.
This plan handles both cases.

## 2. People during a stream

| Role | What they do |
|---|---|
| Manager (on location / host side) | Presents the products. Says which items are worn or held today. |
| CS representative (at H-Global Studio) | **Captures the stream, sets the price and options, publishes.** Answers customers. |
| Viewer / buyer | Watches on YouTube, opens the live section on the site, buys. |

## 3. Listing workflow in H-Global Studio (the core feature)

Target: **under 60 seconds from screenshot to buyable product.**

```
[Capture] -> [Crop] -> [Match?] -> [Price + options] -> [Publish]  => live on the site
```

1. **Capture.** The CS rep presses **Capture** (or a hotkey) while the stream plays in H-Global Studio. One frame is grabbed
   and the **broadcast timestamp is saved with it** (used later for "when it was broadcast").
   Fallbacks: paste an image (Ctrl+V) or upload a file.
2. **Crop.** Quick crop around the product (the frame often includes the manager's hands or face).
3. **Match existing data.** The system searches the catalog for similar product photos and shows the top suggestions.
   - **Match found:** the CS rep taps it and the size, color, quantity and unit price fill in automatically.
   - **No match (new product):** only the essentials are entered: **price (e.g. 149,000 won)** and variations such as size.
     The name defaults to "Live #9" and can be edited now or later. Other details are filled in after the stream.
4. **Publish.** The product appears in the live section for every viewer within a few seconds.
   It can still be edited (price, stock, photos, name) after it goes live.

### Pre-selected vs on-the-fly products

- **Pre-selected:** before the stream, pick up to 8 registered products and number them **No. 1–8** (drag to reorder).
- **On-the-fly:** products captured during the stream get the next number (No. 9, 10, ...).
- Both kinds can be used in the same stream.

### How the capture works (technical options)

| Option | How | Verdict |
|---|---|---|
| A. Screen-capture API in the browser | H-Global Studio asks to share the tab/window that shows the stream and grabs a frame | **Recommended for MVP.** No server video processing, works with any player. |
| B. Grab from the embedded YouTube player | — | Not possible: browsers block reading pixels from the YouTube iframe. |
| C. Server pulls the stream and extracts frames | e.g. ffmpeg on the live stream | Higher quality but heavier, and may conflict with YouTube terms. Consider later. |
| D. Capture from the broadcast PC / encoder (OBS) | Hotkey on the streaming PC sends a frame | Best image quality. Option for later if A is not sharp enough. |

Option A reference: the browser's Screen Capture API, `navigator.mediaDevices.getDisplayMedia()`
([MDN guide](https://developer.mozilla.org/en-US/docs/Web/API/Screen_Capture_API/Using_Screen_Capture),
[MDN getDisplayMedia](https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getDisplayMedia),
[W3C spec](https://w3c.github.io/mediacapture-screen-share/)). The user picks a tab/window/screen once per stream; frames are drawn to a `<canvas>` and saved as an image. Requires HTTPS and a desktop browser.

**Broadcast timestamp:** offset = capture time − stream start time (from the YouTube API), minus a few seconds for delay.
It links to the replay as `youtube.com/watch?v={videoId}&t={offset}s`.

## 4. Live screen on the site (customer side)

- **Red LIVE dot** in the header and on the live section while a stream is on.
- **"Products sold during live streams"** is a section separate from the total inventory.
  - **Top:** items worn or held today (the CS rep pins them).
  - **Below:** the other products from this stream, in order No. 1, 2, 3, ...
  - Newly published items appear without a page reload.
- Video embed plus product list on one screen (mobile first: video on top, products below).
- **Product page:** shows **"Broadcast on 2026-11-03 14:32"** with a button that opens the replay at that moment.
- Uses the earlier site decisions: no Official Price, color swatches, inspection media section.

## 5. After the stream

- Products stay on the site for **N days** after the stream ends, then are **hidden automatically** (not deleted).
  N is not decided yet. Make it a setting per stream (proposed default: 3 days), with an option to keep a product as a permanent listing.
- **KakaoTalk follow-up:** a stream recap page (all products from the stream, with a link to the replay) plus a
  ready-to-send link and share image for the KakaoTalk ad. Sending through the Kakao API directly can come later.
- New products get their full details (description, specs, more photos) added after the stream and are saved to the
  catalog, so next time they **match automatically** (section 3, step 3).

## 6. H-Global Studio screens

1. **Stream setup:** YouTube link, scheduled time, pick No. 1–8, display days after the stream.
2. **Live console (CS rep):** stream view plus Capture button; queue of published items; edit price/stock;
   pin as worn or held today; sold and remaining counts; customer chat.
3. **Quick listing panel:** crop → match suggestions → price/options → Publish.
4. **After the stream:** list of products to finish (missing name/description), recap link for KakaoTalk, expiry dates.
5. **Live products view:** products from streams, filtered separately from total inventory.

## 7. Data model (initial)

- `Show`: youtubeVideoId, scheduledAt, startedAt, endedAt, status (scheduled / live / ended), displayDaysAfter
- `ShowItem`: showId, productId, number (1, 2, ...), source (preselected / captured), pinned (worn or held),
  capturedImage, broadcastOffsetSec, publishedAt, hideAt
- `Product`: draft fields allowed (only image + price required for live items), linked to the master catalog
- `Variant`: size, color, quantity, unit price
- `ImageEmbedding`: productId, vector (for matching screenshots to existing products)
- `Order` / `Reservation`: as in v1 (hold stock during checkout so one-of-a-kind items are not sold twice)

## 8. Phases

| Phase | Scope | Target |
|---|---|---|
| 0 Design | Screens in sections 4 and 6, listing flow | 2026-11-08 |
| 1 MVP | Stream setup, screenshot capture (option A) and paste/upload, quick listing for new products, live section with LIVE dot, auto-hide after N days, "Broadcast on" link | +4–6 weeks |
| 2 Match | Image matching to auto-fill existing products, pinned worn/held items, recap page for KakaoTalk, post-stream completion list | +3–4 weeks |
| 3 Later | Higher-quality capture (option C or D), Kakao API sending, YouTube chat integration, analytics per item | later |

## 9. Open questions

1. How many days should products stay up after a stream? A per-stream setting is proposed, with a default of 3.
2. What is the minimum info for a new product to go live? Proposed: photo + price, with name, options and quantity optional.
3. Is quantity usually known on location, or should we allow "sell until we mark it sold out"?
4. Which device will the CS rep use (PC browser recommended for screen capture)?
5. When a screenshot matches an existing product, should it always be confirmed by the CS rep? (Proposed: yes, never auto-publish.)
6. Which payment provider? Do customers need an account to buy during the stream?
7. Which ClickMate features are used today and must be matched on day one?
