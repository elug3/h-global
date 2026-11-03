# Decision record: not using Clickmate

> Source: 2026-10-31 meeting (payment system, live commerce, customer acquisition)
> Status: Decision as discussed in the meeting; fee comparison not yet verified

## Context
Clickmate (also transcribed as "Clinmate" / "Phinicmake" in the meeting notes) is a
live commerce platform where sellers broadcast and sell through Clickmate's own
checkout. It was raised twice in the meeting:

- As an example of sellers who move between warehouses, capture the broadcast, and
  list newly found products on the site in real time (18:53-19:08).
- As the reason many sellers join such platforms: the checkout window (21:21-22:19).
  One-person channels that sell directly on YouTube also have to confirm every bank
  deposit by hand, which makes the work after the sale heavier than the sale itself.

## Decision
Do not adopt Clickmate. Build the needed functions into our own site instead:

- Real-time product listing during a broadcast (capture a frame, enter price and
  size/color variants, publish immediately).
- Checkout on our own site, so we receive payments ourselves.

## Reasons
1. **Fees.** Using Clickmate means paying its fees, and the team does not want to
   pay them for a checkout it intends to run itself. (22:15-22:46)
2. **Only one function is needed.** The need is the real-time listing function,
   not Clickmate's payment flow. Payment can be received separately. (22:19-22:57)
3. **Scam and impersonation risk.** Once sales grow, scammers copy the business:
   fake KakaoTalk channels, fake sellers collecting orders without shipping, and
   abusive comments in the chat. A checkout inside our own site reduces the need to
   hand out personal bank accounts and limits this exposure. (22:57-23:44)
4. **Name/identity exposure.** Related to the discussion of keeping the business
   owner's personal name off payment and settlement paths (4:34-5:16).

## Open questions
- Clickmate's actual pricing is unverified. Press coverage (Aug 2025) describes no
  commission on sales and an hourly fee based on peak viewers instead, which differs
  from the "fees are deducted" remark in the meeting. The hourly rates were not
  found. Confirm directly before relying on the fee argument.
- No seller reviews of Clickmate's checkout or YouTube integration were found.
- One sentence at 22:04 ("we do not want the checkout window") is garbled in the
  transcript and its speaker is unclear, so it is not used above.

## Related
- Payment decision (own checkout vs PG): not finalized in the meeting.
