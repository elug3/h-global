import { defineConfig, devices } from "@playwright/test";

// End-to-end tests run the storefront against a real hg-server with a fresh
// PostgreSQL 18 database and the fake payment provider. Requires Go and psql.
// Database: HG_E2E_DATABASE_URL (it is wiped at the start).
const db = process.env.HG_E2E_DATABASE_URL ?? "postgres://hglobal:hglobal_dev@localhost:5432/hglobal_e2e?sslmode=disable";
export const SERVER = "http://localhost:18082";
export const SITE = "http://localhost:3102";

export default defineConfig({
  testDir: "e2e",
  timeout: 120_000,
  workers: 1,
  reporter: [["list"]],
  use: {
    ...devices["iPhone 13"],
    browserName: "chromium",
    baseURL: SITE,
    locale: "ko-KR",
    timezoneId: "Asia/Seoul",
    trace: "off",
  },
  webServer: [
    {
      command: `psql "${db}" -q -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" && go run ./cmd/hg-server`,
      cwd: "../hg-server",
      url: `${SERVER}/healthz`,
      timeout: 180_000,
      reuseExistingServer: false,
      env: {
        HG_DATABASE_URL: db,
        HG_HTTP_ADDR: ":18082",
        HG_STORAGE_DIR: "./data/e2e-storefront",
        HG_COOKIE_SECURE: "false",
        HG_OWNER_EMAIL: "owner@hglobal.test",
        HG_OWNER_PASSWORD: "owner-password",
        HG_BCRYPT_COST: "4",
        HG_ALLOWED_ORIGINS: SITE,
        HG_DEV_ROUTES: "true",
      },
    },
    {
      command: "npx react-router dev --port 3102 --strictPort",
      url: SITE,
      timeout: 90_000,
      reuseExistingServer: false,
      env: { HG_API_URL: SERVER },
    },
  ],
});
