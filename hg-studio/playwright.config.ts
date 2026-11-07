import { defineConfig } from "@playwright/test";

// End-to-end tests run hg-studio against a real hg-server with a fresh
// PostgreSQL 18 database and the broadcast relay on. Requires Go, psql and
// ffmpeg. Database: HG_E2E_DATABASE_URL (it is wiped at the start).
const db = process.env.HG_E2E_DATABASE_URL ?? "postgres://hglobal:hglobal_dev@localhost:5432/hglobal_e2e?sslmode=disable";
export const SERVER = "http://localhost:18081";
export const STUDIO = "http://localhost:3101";
export const RELAY = "127.0.0.1:19010";

export default defineConfig({
  testDir: "e2e",
  timeout: 90_000,
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL: STUDIO,
    viewport: { width: 1440, height: 900 },
    locale: "ko-KR",
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
        HG_HTTP_ADDR: ":18081",
        HG_STORAGE_DIR: "./data/e2e-storage",
        HG_COOKIE_SECURE: "false",
        HG_OWNER_EMAIL: "owner@hglobal.test",
        HG_OWNER_PASSWORD: "owner-password",
        HG_BCRYPT_COST: "4",
        HG_ALLOWED_ORIGINS: STUDIO,
        HG_RELAY_ENABLED: "true",
        HG_RELAY_LISTEN: RELAY,
        HG_RELAY_PUBLIC_HOST: RELAY,
        HG_RELAY_SECRET: "e2e-relay-secret-1234",
        HG_RELAY_LOCAL_COPY: "127.0.0.1:19011",
      },
    },
    {
      command: "npx vite --port 3101 --strictPort",
      url: STUDIO,
      timeout: 60_000,
      reuseExistingServer: false,
      env: { HG_SERVER_URL: SERVER },
    },
  ],
});
