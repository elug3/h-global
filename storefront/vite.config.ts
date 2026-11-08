import { reactRouter } from "@react-router/dev/vite";
import { defineConfig } from "vite";

// In development the browser talks to this server only; /api and /media are
// forwarded to hg-server (HG_API_URL). In production the TLS proxy (Caddy)
// does the same, so cookies stay first-party.
const api = process.env.HG_API_URL ?? "http://localhost:8080";

export default defineConfig({
  plugins: [reactRouter()],
  server: {
    port: Number(process.env.PORT ?? 3000),
    proxy: {
      "/api": { target: api, changeOrigin: false },
      "/media": { target: api, changeOrigin: false },
    },
  },
});
