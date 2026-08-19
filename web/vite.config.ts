/// <reference types="vitest/config" />
import { sveltekit } from "@sveltejs/kit/vite";
import tailwindcss from "@tailwindcss/vite";
import { svelteTesting } from "@testing-library/svelte/vite";
import { defineConfig } from "vite";

const proxyTarget = process.env.DEV_API_PROXY || "http://localhost:8080";

export default defineConfig({
  plugins: [tailwindcss(), sveltekit(), svelteTesting()],
  // Component tests run through vitest (bunx vitest run) using this
  // config; `bun test` runs the same files via bunfig.toml's preload.
  test: {
    environment: "jsdom",
  },
  server: {
    host: "0.0.0.0",
    port: Number(process.env.VITE_PORT) || 5173,
    proxy: {
      "/api": {
        target: proxyTarget,
        changeOrigin: true,
      },
    },
  },
});
