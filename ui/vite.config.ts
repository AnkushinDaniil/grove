import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev server proxies API/WS calls to the grove daemon so `npm run dev` can
// talk to a real `grove serve` instance running on its default port. Mock
// mode (`VITE_MOCK=1`, see src/mock/) bypasses this entirely.
const DAEMON_ORIGIN = "http://127.0.0.1:7433";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api": {
        target: DAEMON_ORIGIN,
        changeOrigin: true,
      },
      "/ws": {
        target: DAEMON_ORIGIN,
        ws: true,
        changeOrigin: true,
      },
    },
  },
});
