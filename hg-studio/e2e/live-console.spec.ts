import { spawn, type ChildProcess } from "node:child_process";
import { expect, test, type Page } from "@playwright/test";

const shots = process.env.HG_SCREENSHOT_DIR;
async function shot(page: Page, name: string) {
  if (shots) await page.screenshot({ path: `${shots}/${name}.png` });
}

/** A test broadcast like OBS would send: a moving test pattern over SRT. */
function broadcast(url: string): ChildProcess {
  return spawn(
    "ffmpeg",
    [
      "-hide_banner", "-loglevel", "error", "-re",
      "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30",
      "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
      "-t", "60", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", "30",
      "-pix_fmt", "yuv420p", "-c:a", "aac", "-f", "mpegts", url,
    ],
    { stdio: "ignore" },
  );
}

test("stream setup to quick listing in the live console", async ({ page }) => {
  // 1. Sign in.
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "H-Global Studio 로그인" })).toBeVisible();
  await page.getByLabel("이메일").fill("owner@hglobal.test");
  await page.getByLabel("비밀번호").fill("wrong-password");
  await page.getByRole("button", { name: "로그인" }).click();
  await expect(page.getByRole("alert")).toHaveText("이메일 또는 비밀번호가 맞지 않습니다.");
  await page.getByLabel("비밀번호").fill("owner-password");
  await page.getByRole("button", { name: "로그인" }).click();
  await expect(page.getByRole("heading", { name: "방송", exact: true })).toBeVisible();

  // 2. Create a stream and prepare it.
  await page.getByLabel("새 방송 만들기").fill("동대문 창고 투어 — 가방 & 뷰티");
  await page.getByRole("button", { name: "새 방송" }).click();
  await expect(page.getByRole("heading", { name: "방송 정보" })).toBeVisible();
  await page.getByLabel("YouTube 링크").fill("https://youtu.be/dQw4w9WgXcQ");
  await page.getByLabel("진행자 (현장)").fill("민준");
  await page.getByText("방송 예정으로 공개").click();
  await page.getByRole("button", { name: "저장" }).click();
  await expect(page.getByText("저장했습니다")).toBeVisible();

  // A preselected product, made on the spot: it becomes No.1.
  await page.getByText("새 상품 만들기").click();
  await page.getByLabel("상품 이름").fill("퀼팅 체인백");
  await page.getByLabel("가격 (원)").fill("149000");
  await page.getByRole("button", { name: "추가", exact: true }).click();
  await expect(page.locator(".pre-row")).toHaveCount(1);
  await expect(page.locator(".pre-row")).toContainText("퀼팅 체인백");

  const ingest = (await page.getByTestId("ingest-url").textContent()) ?? "";
  expect(ingest).toMatch(/^srt:\/\/127\.0\.0\.1:19010\?passphrase=[A-Z2-7]{32}&latency=300000$/);
  await shot(page, "1-stream-setup");

  // 3. Go live and open the console.
  page.once("dialog", (d) => void d.accept());
  await page.getByRole("button", { name: "방송 시작" }).click();
  await expect(page.getByTestId("relay-status")).toBeVisible();
  await expect(page.getByTestId("relay-status")).toHaveText("송출 대기 중");

  const pub = broadcast(ingest);
  try {
    await expect(page.getByTestId("relay-status")).toHaveText("수신 중", { timeout: 30_000 });
    await expect(page.locator(".preview img")).toBeVisible();

    // Capture with the Space key, then crop by dragging.
    await page.locator("body").click({ position: { x: 5, y: 5 } });
    await page.keyboard.press("Space");
    await expect(page.locator(".capture")).toHaveCount(1);
    const crop = page.getByTestId("crop");
    await expect(crop.locator("img")).toBeVisible();
    const box = (await crop.boundingBox())!;
    await page.mouse.move(box.x + box.width * 0.25, box.y + box.height * 0.2);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width * 0.75, box.y + box.height * 0.85, { steps: 5 });
    await page.mouse.up();
    await expect(crop.locator(".crop-rect")).toBeVisible();

    // Price and options, then publish as No.2.
    await page.getByLabel("가격", { exact: true }).fill("89000");
    await page.getByLabel("새 색상 이름").fill("브라운");
    await page.getByLabel("새 색상 이름").press("Enter");
    await page.getByLabel("새 색상 이름").fill("블랙");
    await page.getByLabel("새 색상 이름").press("Enter");
    await page.getByLabel("새 사이즈").fill("미니");
    await page.getByLabel("새 사이즈").press("Enter");
    await page.getByText("수량 모름 — 품절 처리 전까지 판매").click();
    await shot(page, "2-quick-listing");
    await page.getByRole("button", { name: "No.2 등록" }).click();
    await expect(page.getByText("No.2 등록 완료")).toBeVisible();

    const list = page.getByTestId("published");
    await expect(list.locator(".pub-row")).toHaveCount(2);
    await expect(list.locator(".pub-row").first()).toContainText("Live #2");
    await expect(list.locator(".pub-row").first()).toContainText("₩89,000");

    // Pin the new item as worn or held today.
    await list.locator(".pub-row").first().getByRole("button", { name: "오늘 착용/소지 표시" }).click();
    await expect(list.locator(".pub-row").first().getByRole("button", { name: "표시 해제" })).toBeVisible();
    await shot(page, "3-live-console");

    // The form resets, and the colors typed are now suggested, not selected.
    const brown = page.locator(".chip", { hasText: "브라운" });
    await expect(brown).toBeVisible();
    await expect(brown).toHaveAttribute("aria-pressed", "false");
    await page.keyboard.press("Space");
    await expect(page.locator(".capture")).toHaveCount(2);
    await expect(page.getByRole("button", { name: "No.3 등록" })).toBeVisible();
  } finally {
    pub.kill("SIGKILL");
  }

  // 4. End the stream.
  page.once("dialog", (d) => void d.accept());
  await page.getByRole("button", { name: "방송 종료" }).click();
  await expect(page.getByText("방송이 종료되었습니다.")).toBeVisible();
});
