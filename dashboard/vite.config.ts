import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the dashboard runs on Vite's dev server and proxies API
// calls to the control plane. In production the control plane serves the
// built files from dist/ itself.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": process.env.ORKESTRA_SERVER ?? "http://localhost:8080",
    },
  },
});
