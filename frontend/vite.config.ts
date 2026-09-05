/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), wails("./bindings")],
  build: {
    chunkSizeWarningLimit: 900,
    // NOTE: vendor-level advancedChunks (react/antd split) is deliberately NOT
    // used: rolldown splits break module-eval order for the react<->antd
    // circular imports, producing a blank window.
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    // The wails vite plugin generates binding shims at dev/build time; the
    // tests import the checked-in bindings directly, so skip it in vitest.
    plugins: () => [react()],
  },
});
