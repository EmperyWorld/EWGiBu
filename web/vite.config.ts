import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Deterministic asset names; no build-time path injection into bundles.
export default defineConfig({
  plugins: [react()],
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    assetsInlineLimit: 0,
    rollupOptions: {
      output: {
        entryFileNames: "assets/index.js",
        chunkFileNames: "assets/[name].js",
        assetFileNames: "assets/[name][extname]",
      },
    },
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:7432",
      "/openapi.yaml": "http://127.0.0.1:7432",
    },
  },
});
