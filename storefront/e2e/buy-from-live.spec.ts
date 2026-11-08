import { expect, request, test, type APIRequestContext, type Page } from "@playwright/test";
import { SERVER } from "../playwright.config";

const shots = process.env.HG_SCREENSHOT_DIR;
/** Waits until React has taken over the server-rendered page. */
async function ready(page: Page) {
  await page.locator("html[data-ready]").waitFor({ state: "attached" });
}

async function shot(page: Page, name: string) {
  if (shots) await page.screenshot({ path: `${shots}/${name}.png` });
}

type Json = Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any

/** Calls hg-server directly as staff (what H-Global Studio does). */
async function call(ctx: APIRequestContext, method: string, path: string, data?: unknown, multipart?: Json): Promise<Json> {
  const res = await ctx.fetch(path, { method, data, multipart });
  expect(res.ok(), `${method} ${path}: ${res.status()} ${await res.text()}`).toBeTruthy();
  return res.status() === 204 ? {} : res.json();
}

/** Staff list a product on air from a captured frame, like the live console. */
async function listOnAir(staff: APIRequestContext, showId: string, png: Buffer, body: Json): Promise<Json> {
  const capture = await call(staff, "POST", `/studio-api/shows/${showId}/captures`, undefined, {
    file: { name: "frame.png", mimeType: "image/png", buffer: png },
  });
  return call(staff, "POST", `/studio-api/shows/${showId}/items/quick`, { capture_id: capture.id, ...body });
}

test("watch the stream, buy an item, follow the order to the door", async ({ page, browser }) => {
  const staff = await request.newContext({ baseURL: SERVER });
  await call(staff, "POST", "/studio-api/auth/login", { email: "owner@hglobal.test", password: "owner-password" });
  // A colorful test image stands in for camera frames and inspection photos.
  await page.setContent('<div style="height:100vh;background:linear-gradient(135deg,#c9a27a,#6b4f3a 45%,#1d1d1d)"></div>');
  const png = await page.screenshot();

  // 1. A stream is announced ("방송 예정으로 공개"): home shows the next-stream card.
  const show = await call(staff, "POST", "/studio-api/shows", {
    title: "동대문 창고 투어 — 가방 & 뷰티",
    youtube_url: "https://youtu.be/dQw4w9WgXcQ",
    scheduled_at: new Date(Date.now() + 2 * 3600_000).toISOString(),
    status: "scheduled",
  });
  await page.goto("/");
  await ready(page);
  await expect(page.getByTestId("next-show")).toContainText("동대문 창고 투어");
  await expect(page.getByTestId("next-show")).toContainText("다음 방송");
  await shot(page, "1-home-next");

  // 2. It goes live and the first item is listed on air.
  await call(staff, "POST", `/studio-api/shows/${show.id}/start`);
  const bag = await listOnAir(staff, show.id, png, {
    title: "퀼팅 체인백",
    price_won: 149000,
    options: [{ name: "색상", values: [{ label: "블랙", hex1: "#111111" }, { label: "아이보리", hex1: "#F3EDE1", hex2: "#111111" }] }],
    stock: { mode: "counted", qty: 1 },
  });
  await page.reload();
  await ready(page);
  await expect(page.getByTestId("live-banner")).toContainText("방송 중 · 지금까지 1개");
  await page.getByRole("link", { name: /보면서 쇼핑하기/ }).click();
  await expect(page.getByRole("heading", { name: "동대문 창고 투어 — 가방 & 뷰티" })).toBeVisible();
  const items = page.getByTestId("items");
  await expect(items.getByTestId("item-1")).toContainText("₩149,000");

  // 3. The next item appears without reloading, with a notice.
  await listOnAir(staff, show.id, png, { price_won: 89000, stock: { mode: "until_sold_out", qty: 0 } });
  await expect(items.getByTestId("item-2")).toContainText("Live #2");
  await expect(items.getByTestId("item-2")).toContainText("NEW");
  await expect(page.getByRole("button", { name: /No.2 방금 등록/ })).toBeVisible();
  await shot(page, "2-live-page");

  // 4. No.1 opens the product sheet over the stream; a color must be chosen.
  await items.getByTestId("item-1").click();
  const sheet = page.getByRole("dialog", { name: "퀼팅 체인백" });
  await expect(sheet).toContainText("2개 남음"); // one per color
  await sheet.getByRole("button", { name: /바로 구매/ }).click();
  await expect(sheet.getByRole("alert")).toHaveText("색상을(를) 골라 주세요");
  await sheet.getByRole("radio", { name: "아이보리" }).click();
  await expect(sheet).toContainText("1개 남음");
  await shot(page, "3-sheet");
  await sheet.getByRole("button", { name: /바로 구매 · ₩149,000/ }).click();

  // 5. Not signed in: sign up, then back to the product with the same choice.
  await expect(page.getByRole("heading", { name: "회원가입" })).toBeVisible();
  await expect(page.getByText("퀼팅 체인백을(를) 구매하려면")).toBeVisible();
  await page.getByLabel("이름").fill("김민아");
  await page.getByLabel("이메일").fill("mina@example.com");
  await page.getByLabel("비밀번호").fill("mina-password");
  await page.getByText("[필수] 이용약관").click();
  await page.getByRole("button", { name: "가입하고 계속하기" }).click();
  await expect(page.getByRole("heading", { name: "퀼팅 체인백" })).toBeVisible();
  await expect(page.getByRole("radio", { name: "아이보리" })).toHaveAttribute("aria-checked", "true");
  await page.getByRole("button", { name: /바로 구매 · ₩149,000/ }).click();

  // 6. Checkout holds the bag; first time, the address form; the customs code is checked.
  await expect(page.getByRole("heading", { name: "주문/결제" })).toBeVisible();
  await expect(page.getByRole("timer")).toContainText("구매자님을 위해 확보 중");
  await page.getByLabel("받는 분").fill("김민아");
  await page.getByLabel("휴대폰 번호").fill("010-1234-5678");
  await page.getByLabel("우편번호", { exact: true }).fill("04524");
  await page.getByLabel("기본 주소").fill("서울특별시 중구 세종대로 110");
  await page.getByLabel("상세 주소 (동, 호수, 층)").fill("5층");
  await page.getByLabel("개인통관고유부호").fill("P12345");
  await expect(page.getByText("P와 숫자 12자리를 입력해 주세요.")).toBeVisible();
  await page.getByLabel("개인통관고유부호").fill("p123456789012");
  await expect(page.getByText("형식 확인")).toBeVisible();
  await shot(page, "4-address");
  await page.getByRole("button", { name: "저장" }).click();
  await expect(page.getByTestId("address")).toContainText("(04524) 서울특별시 중구 세종대로 110 5층");
  await expect(page.getByText("P123456789012")).toBeVisible();
  await shot(page, "5-checkout");
  await page.getByRole("button", { name: "₩149,000 결제하기" }).click();

  // 7. The fake provider's page stands in for the card page; approve.
  await expect(page.getByRole("heading", { name: "테스트 결제" })).toBeVisible();
  await page.getByRole("button", { name: "결제 승인" }).click();
  await expect(page.getByTestId("paid")).toContainText("결제가 완료되었습니다");
  const orderNo = (await page.getByTestId("paid").locator(".mono").textContent()) ?? "";
  expect(orderNo).toMatch(/^HG-\d{6}-[23456789BCDFGHJKMNPQRSTVWXZ]{5}$/);
  await shot(page, "6-paid");

  // The one-of-a-kind ivory bag is now sold out for everyone; black is still on sale.
  const viewer = await browser.newPage();
  await viewer.goto(`/shows/${show.id}`);
  await ready(viewer);
  await viewer.getByTestId("item-1").click();
  await expect(viewer.getByRole("radio", { name: "아이보리 · 품절" })).toBeDisabled();
  await expect(viewer.getByRole("radio", { name: "블랙" })).toBeEnabled();
  await viewer.close();

  // 8. Staff inspect and check the customs code; the buyer sees the photos.
  const orders = (await call(staff, "GET", "/studio-api/orders")).items as Json[];
  const order = orders.find((o) => o.number === orderNo)!;
  await call(staff, "POST", `/studio-api/orders/${order.id}/inspection`, undefined, {
    kind: "photo",
    file: { name: "inspection.png", mimeType: "image/png", buffer: png },
  });
  await call(staff, "POST", `/studio-api/orders/${order.id}/inspection/complete`, { note: "방송 속 상품과 같음" });
  await page.getByRole("link", { name: "주문 상세 보기" }).click();
  await expect(page.locator(".odh")).toContainText(orderNo);
  await expect(page.locator(".odh")).toContainText("통관 확인");
  await expect(page.getByTestId("inspection").locator("img")).toHaveCount(1);
  await expect(page.getByText("상품 준비가 시작되어 직접 취소할 수 없습니다")).toBeVisible();
  await shot(page, "7-order-detail");

  // 9. Shipped and delivered; the buyer confirms receipt.
  await call(staff, "POST", `/studio-api/orders/${order.id}/customs-check`);
  await call(staff, "POST", `/studio-api/orders/${order.id}/ship`, { carrier: "CJ대한통운", tracking_number: "6012-3456-7890" });
  await call(staff, "POST", `/studio-api/orders/${order.id}/deliver`);
  await page.reload();
  await ready(page);
  await expect(page.getByText("CJ대한통운 6012-3456-7890")).toBeVisible();
  page.once("dialog", (d) => void d.accept());
  await page.getByRole("button", { name: "수령 확인" }).click();
  await expect(page.locator(".odh")).toContainText("구매 확정");

  // 10. A second item by virtual account: the account to pay into, then cancel.
  await page.goto(`/shows/${show.id}`);
  await ready(page);
  await page.getByTestId("item-2").click();
  await page.getByRole("dialog").getByRole("button", { name: /바로 구매 · ₩89,000/ }).click();
  await expect(page.getByTestId("address")).toContainText("김민아"); // saved on this device
  await page.getByRole("radio", { name: "가상계좌" }).click();
  await page.getByRole("button", { name: "₩89,000 결제하기" }).click();
  await expect(page.getByTestId("va")).toContainText("Fake Bank");
  await expect(page.getByTestId("va")).toContainText("₩89,000");
  await shot(page, "8-virtual-account");
  await page.goto("/orders");
  await ready(page);
  const cards = page.getByTestId("orders").locator(".ocard");
  await expect(cards).toHaveCount(2);
  await expect(cards.first()).toContainText("결제 대기");
  await shot(page, "9-my-orders");
  page.once("dialog", (d) => void d.accept());
  await cards.first().getByRole("button", { name: "주문 취소" }).click();
  await expect(cards.first()).toContainText("취소");
  await expect(cards.nth(1)).toContainText("구매 확정");

  // 11. Product links carry a title, price and photo in the server-rendered HTML.
  const html = await (await page.request.get(`/products/${bag.product_id}`)).text();
  expect(html).toContain('<meta property="og:title" content="퀼팅 체인백 ₩149,000"');
  expect(html).toMatch(/<meta property="og:image" content="http:\/\/localhost:3102\/media\/[^"]+"/);

  // 12. Desktop: video on the left, items on the right (board 6).
  const desk = await browser.newPage({ viewport: { width: 1440, height: 900 }, locale: "ko-KR", timezoneId: "Asia/Seoul" });
  await desk.goto(`/shows/${show.id}`);
  await ready(desk);
  await expect(desk.getByTestId("item-2")).toBeVisible();
  if (shots) await desk.screenshot({ path: `${shots}/10-desktop-live.png` });
  await desk.close();

  await staff.dispose();
});
