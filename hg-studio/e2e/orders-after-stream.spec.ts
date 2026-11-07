import { expect, request, test, type APIRequestContext, type Page } from "@playwright/test";
import { SERVER } from "../playwright.config";

const shots = process.env.HG_SCREENSHOT_DIR;
async function shot(page: Page, name: string) {
  if (shots) await page.screenshot({ path: `${shots}/${name}.png` });
}

type Json = Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any

/** Calls hg-server directly (setup only; the screens are driven in the browser). */
async function call(ctx: APIRequestContext, method: string, path: string, data?: unknown): Promise<Json> {
  const res = await ctx.fetch(path, { method, data });
  expect(res.ok(), `${method} ${path}: ${res.status()} ${await res.text()}`).toBeTruthy();
  return res.status() === 204 ? {} : res.json();
}

let customers = 0;
/** A new customer buys one line and, if pay, the fake provider approves it. */
async function buy(variantId: string, qty: number, recipient: string, pay: boolean): Promise<Json> {
  const ctx = await request.newContext({ baseURL: SERVER });
  customers++;
  await call(ctx, "POST", "/api/auth/register", { email: `buyer${customers}@example.com`, password: "buyer-password", name: recipient });
  const co = await call(ctx, "POST", "/api/checkout", { lines: [{ variant_id: variantId, qty }] });
  await call(ctx, "PUT", `/api/checkout/${co.id}/shipping`, {
    recipient,
    phone: "010-1234-5678",
    postcode: "04524",
    address1: "서울특별시 중구 세종대로 110",
    address2: "5층",
    pccc: "P123456789012",
  });
  const res = await call(ctx, "POST", `/api/checkout/${co.id}/pay`, { method: "card" });
  if (pay) await call(ctx, "POST", res.payment.redirect_url);
  await ctx.dispose();
  return res.order;
}

/** A link in the left menu. */
function menu(page: Page, name: string) {
  return page.getByRole("navigation", { name: "메뉴" }).getByRole("link", { name: new RegExp(`^${name}`) });
}

async function signIn(page: Page) {
  await page.goto("/login");
  await page.getByLabel("이메일").fill("owner@hglobal.test");
  await page.getByLabel("비밀번호").fill("owner-password");
  await page.getByRole("button", { name: "로그인" }).click();
  await expect(page.getByRole("heading", { name: "방송", exact: true })).toBeVisible();
}

test("orders from a stream, then the after-stream screen, products and staff", async ({ page }) => {
  // Setup through the API: two products in a stream, three orders, stream ended.
  const staff = await request.newContext({ baseURL: SERVER });
  await call(staff, "POST", "/studio-api/auth/login", { email: "owner@hglobal.test", password: "owner-password" });
  const tote = await call(staff, "POST", "/studio-api/products", {
    title: "레더 토트백",
    price_won: 189000,
    options: [{ name: "색상", values: [{ label: "브라운", hex1: "#6b4f3a" }, { label: "블랙", hex1: "#111111" }] }],
    stock: { mode: "counted", qty: 3 },
  });
  const wallet = await call(staff, "POST", "/studio-api/products", {
    title: "카드 지갑",
    description: "소가죽 카드 지갑",
    price_won: 49000,
    stock: { mode: "counted", qty: 10 },
  });
  const show = await call(staff, "POST", "/studio-api/shows", {
    title: "주말 특가 라이브",
    youtube_url: "https://youtu.be/dQw4w9WgXcQ",
    status: "scheduled",
  });
  for (const p of [tote, wallet]) await call(staff, "POST", `/studio-api/shows/${show.id}/items`, { product_id: p.id });
  await call(staff, "POST", `/studio-api/shows/${show.id}/start`);
  const brown = tote.variants.find((v: Json) => v.label === "브라운").id;
  const walletV = wallet.variants[0].id;
  await buy(brown, 1, "김민아", true);
  await buy(walletV, 2, "이준호", true);
  await buy(walletV, 1, "최유나", false);
  await call(staff, "POST", `/studio-api/shows/${show.id}/end`);

  // 1. Orders: two waiting for staff, one unpaid.
  await signIn(page);
  await expect(page.locator(".rail-badge")).toHaveText("2");
  await menu(page, "주문").click();
  await expect(page.getByRole("heading", { name: "주문", exact: true })).toBeVisible();
  await expect(page.getByRole("tab", { name: /처리 필요/ })).toHaveAttribute("aria-selected", "true");
  const rows = page.getByTestId("orders").locator("tbody tr");
  await expect(rows).toHaveCount(2);

  // A new paid order appears without reloading.
  await buy(walletV, 1, "박지훈", true);
  await expect(rows).toHaveCount(3);
  await expect(rows.first()).toContainText("박지훈");
  await expect(page.getByRole("tab", { name: /처리 필요/ })).toContainText("3");

  await page.getByRole("tab", { name: /결제 대기/ }).click();
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText("최유나");
  await page.getByRole("tab", { name: /처리 필요/ }).click();
  await expect(rows).toHaveCount(3);
  await expect(page.locator(".rail-badge")).toHaveText("3"); // the menu badge follows too
  await shot(page, "4-orders");

  // 2. One order through inspection, customs check, shipping and delivery.
  await page.getByLabel("주문 검색").fill("김민아");
  await page.getByRole("button", { name: "검색" }).click();
  await expect(rows).toHaveCount(1);
  await rows.first().getByRole("link").click();
  await expect(page.getByRole("heading", { name: /^주문 HG-\d{6}-\d{5}$/ })).toBeVisible();
  const status = page.locator(".page-head .tag");
  const next = page.getByTestId("next-step");
  await expect(status).toHaveText("결제 완료");
  await expect(next.getByRole("heading", { name: "검수" })).toBeVisible();
  await expect(page.getByText("레더 토트백")).toBeVisible();

  const photo = await page.screenshot(); // any real PNG will do as an inspection photo
  await page.getByTestId("inspection-file").setInputFiles({ name: "inspection.png", mimeType: "image/png", buffer: photo });
  await expect(status).toHaveText("검수 중");
  await expect(next.locator(".gallery img")).toHaveCount(1);
  await page.getByLabel("검수 메모 (선택)").fill("방송 속 상품과 같음, 흠집 없음");
  await shot(page, "5-order-inspection");
  await page.getByRole("button", { name: "검수 완료" }).click();

  await expect(status).toHaveText("통관 확인");
  await expect(next.getByText("P123456789012")).toBeVisible();
  await page.getByRole("button", { name: "통관부호 확인 완료" }).click();
  await expect(next.getByText(/통관부호 확인됨/)).toBeVisible();
  await page.getByLabel("송장번호").fill("6012-3456-7890");
  await page.getByRole("button", { name: "배송 시작" }).click();
  await expect(status).toHaveText("배송 중");
  await expect(next).toContainText("CJ대한통운 6012-3456-7890");
  await page.getByRole("button", { name: "배송 완료 처리" }).click();
  await expect(status).toHaveText("배송 완료");
  await page.getByRole("button", { name: "구매 확정 처리" }).click();
  await expect(status).toHaveText("구매 확정");
  await expect(next).toContainText("처리가 끝난 주문입니다.");
  const history = page.getByTestId("history");
  await expect(history).toContainText("검수 중 → 통관 확인");
  await expect(history).toContainText("방송 속 상품과 같음, 흠집 없음");
  await expect(history).toContainText("통관부호 확인");
  await expect(page.locator(".progress li.done")).toHaveCount(6);

  // 3. Cancel and refund another order.
  await menu(page, "주문").click();
  await page.getByLabel("주문 검색").fill("이준호");
  await page.getByRole("button", { name: "검색" }).click();
  await expect(rows).toHaveCount(1);
  await rows.first().getByRole("link").click();
  await page.getByRole("button", { name: "주문 취소" }).click();
  await expect(page.getByText("₩98,000을 전액 환불하고 재고를 되돌립니다.")).toBeVisible();
  await page.getByLabel("취소 사유").fill("고객 요청");
  await page.getByRole("button", { name: "취소하고 환불" }).click();
  await expect(status).toHaveText("취소");
  await expect(page.getByText("−₩98,000")).toBeVisible();

  // 4. After the stream: sales, keep listed, sale period, details to fill, recap.
  await menu(page, "방송").click();
  await page.locator(".show-row", { hasText: "주말 특가 라이브" }).getByRole("link", { name: "방송 후 정리" }).click();
  await expect(page.getByRole("heading", { name: "주말 특가 라이브" })).toBeVisible();
  const stats = page.getByTestId("stats");
  await expect(stats.locator(".stat", { hasText: "판매액" })).toContainText("₩238,000");
  await expect(stats.locator(".stat", { hasText: /^주문/ })).toContainText("2");
  await expect(stats.locator(".stat", { hasText: "결제 대기" })).toContainText("1");
  await expect(stats.locator(".stat", { hasText: "취소·환불" })).toContainText("1");
  const items = page.getByTestId("after-items").locator("tbody tr");
  await expect(items).toHaveCount(2);
  await expect(items.nth(0)).toContainText("₩189,000");
  await expect(items.nth(1)).toContainText("₩49,000");

  await page.getByLabel("No.1 계속 판매").click({ force: true }); // changes once hg-server answers
  await expect(page.getByLabel("No.1 계속 판매")).toBeChecked();
  await expect(items.nth(0)).toContainText("기간 없음");
  const period = page.getByRole("group", { name: "방송 후 판매 기간" });
  await period.getByRole("button", { name: "+" }).click();
  await period.getByRole("button", { name: "+" }).click();
  await expect(period).toContainText("5일");
  await page.getByRole("button", { name: "저장" }).click();
  await expect(page.getByText("저장했습니다")).toBeVisible();

  const recap = page.getByTestId("recap");
  await expect(recap).toHaveValue(/\[H-Global\] 주말 특가 라이브 다시보기/);
  await expect(recap).toHaveValue(/youtube\.com\/watch\?v=dQw4w9WgXcQ/);
  await expect(recap).toHaveValue(/No\.1 레더 토트백 ₩189,000/);
  const completion = page.getByTestId("completion");
  await expect(completion).toContainText("레더 토트백");
  await expect(completion).not.toContainText("카드 지갑");
  await shot(page, "6-after-stream");

  // 5. Fill in the product: description, a spec, a photo, more stock.
  await completion.getByRole("link", { name: "정보 입력" }).click();
  await expect(page.getByRole("heading", { name: "레더 토트백" })).toBeVisible();
  await expect(page.getByTestId("missing")).toContainText("설명");
  await page.getByLabel("설명").fill("부드러운 소가죽 토트백. 노트북 13인치가 들어갑니다.");
  await page.getByLabel("항목", { exact: true }).fill("소재");
  await page.getByLabel("내용", { exact: true }).fill("소가죽");
  await page.getByRole("form", { name: "기본 정보" }).getByRole("button", { name: "저장" }).click();
  await expect(page.getByText("저장했습니다")).toBeVisible();
  await expect(page.getByTestId("missing")).toHaveCount(0);

  await page.getByTestId("photos-file").setInputFiles({ name: "tote.png", mimeType: "image/png", buffer: photo });
  await expect(page.getByTestId("photos").locator(".gallery img")).toHaveCount(1);
  await expect(page.getByTestId("photos")).toContainText("대표");

  const black = page.getByTestId("variants").locator("tr", { hasText: "블랙" });
  await black.getByLabel("재고", { exact: true }).fill("5");
  await black.getByRole("button", { name: "블랙 저장" }).click();
  await expect(black.locator("td").nth(5)).toHaveText("5");
  await shot(page, "7-product-edit");

  await page.getByRole("link", { name: "닫기" }).click();
  await expect(page.getByTestId("completion")).toContainText("모든 상품의 정보가 입력되었습니다.");

  // 6. Products list: the incomplete filter no longer shows the tote.
  await menu(page, "상품").click();
  await expect(page.getByTestId("products")).toContainText("레더 토트백");
  await page.getByText("정보 미완성만").click();
  await expect(page.getByTestId("products").or(page.getByText("조건에 맞는 상품이 없습니다."))).toBeVisible();
  await expect(page.getByText("레더 토트백")).toHaveCount(0);

  // 7. Staff: add a CS member and promote them.
  await menu(page, "직원").click();
  await page.getByLabel("이름").fill("이서연");
  await page.getByLabel("이메일").fill("seoyeon@hglobal.test");
  await page.getByLabel("임시 비밀번호").fill("seoyeon-password");
  await page.getByRole("button", { name: "직원 추가" }).click();
  await expect(page.getByText("직원을 추가했습니다.")).toBeVisible();
  const seoyeon = page.getByTestId("staff").locator("tr", { hasText: "이서연" });
  await seoyeon.getByLabel("이서연 역할").selectOption("manager");
  await expect(seoyeon.getByLabel("이서연 역할")).toHaveValue("manager");
  await shot(page, "8-staff");

  await staff.dispose();
});
