import { defineConfig } from "@playwright/test";

const externalBase = process.env.IACPACK_E2E_BASE;

export default defineConfig({
  testDir: "./e2e",
  timeout: 120_000,
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  use: {
    baseURL: externalBase ?? "http://127.0.0.1:7432",
    trace: "on-first-retry",
  },
  ...(externalBase
    ? {}
    : {
        webServer: {
          command: "node scripts/e2e-server.mjs",
          url: "http://127.0.0.1:7432/api/v1/health",
          reuseExistingServer: !process.env.CI,
          timeout: 120_000,
        },
      }),
});
