# Real-time product listing during live broadcasts

> Source: 2026-10-31 meeting (transcript 12:39-20:52)
> Status: Requested in the meeting; build not confirmed. Scope and owner undecided.

## Summary
During a live broadcast, staff capture a frame of the product being shown, enter a
price (and size/color variants), and the product appears on the site immediately so
viewers can buy it. Requested by Speaker 4; the original design by Speakers 1 and 3
already covered most of it.

## Why it is needed
1. **Products that are new on site.** (17:47-19:45, Speaker 4)
   - For products already registered, the host only needs to pick a running order.
   - On market or warehouse broadcasts, unfamiliar new products show up that have no
     registration and no data. The host may know only the category (e.g. cosmetics)
     until arriving, so these cannot be registered in advance.
2. **Buy what is being shown, right now.** (15:40) The goal is that a product being
   promoted on air becomes purchasable immediately.
3. **Keep selling after the broadcast.** (20:33-20:52) Products stay up for a few days
   after the broadcast ends, and regular customers are re-promoted to on KakaoTalk.
4. **Avoid paying for a platform.** (18:33-19:08, 22:15-22:46) Platforms such as
   Clickmate already offer this; the team wants the function on its own site without
   those fees. See [clickmate.md](clickmate.md).

## Required functions
**Listing flow**
- Capture a broadcast frame, set the price, and publish to the site immediately
  (example price in the meeting: 149,000 KRW). (14:59-15:10)
- During a broadcast a manager and a CS agent sit together; the CS agent does the
  capture and upload. (13:09, 20:11)
- If supplier data was received in advance, the capture is linked to it, so size,
  color, quantity and unit price are already filled in. (19:45-20:33, Speaker 6)
- If no data exists, only the variants (e.g. sizes) need to be entered. (16:37)

**Live view** (part of the original design by Speakers 1 and 3; 13:45-15:22, 17:19-17:47)
- A red dot marks that a broadcast is live.
- A set of products for the session (numbered 1-8) can be prepared as candidates.
  Capturing and listing during the broadcast without presetting is also allowed.
- A separate "on air" section; products worn or held today appear at the top, other
  products below.
- Products shown in the broadcast can be viewed separately from total inventory.
- A product page shows when it was broadcast and links back to that broadcast.

**After the broadcast**
- Keep products visible for a few days, then take them down. The number of days is
  not decided.

## Points of disagreement in the meeting
- The function was in the original plan, then removed because Speaker 2 said she
  would send photos and register products herself (13:25-14:30). Speaker 3 said it
  could be added back.
- Speaker 4: if the purpose is selling during broadcasts, this approach is right; if
  the site only stays up for promotion, the current approach is enough (13:32).
- Speaker 3: h-global can be built Dupli-like, but live commerce should have its own
  merit, so h-global will take a different approach (16:26).

## Open questions
- Is this being built, and when? No decision or date was given in the meeting.
- How many days products stay visible after a broadcast.
- Sentence at 15:12 is garbled in the transcript; it may be about returns, so it is
  not used above.
