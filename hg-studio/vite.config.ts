import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development Vite forwards the API to hg-server on the same origin, so
// the staff session cookie stays first-party (plan 3.3). In production the
// TLS proxy (Caddy) does the same.
const server = process.env.HG_SERVER_URL ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    port: Number(process.env.PORT ?? 3001),
    proxy: {
      "/studio-api": { target: server, changeOrigin: false },
      "/media": { target: server, changeOrigin: false },
    },
  },
  preview: {
    port: Number(process.env.PORT ?? 3001),
    proxy: {
      "/studio-api": { target: server, changeOrigin: false },
      "/media": { target: server, changeOrigin: false },
    },
  },
});
