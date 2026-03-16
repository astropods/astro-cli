import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  use: {
    baseURL: "http://127.0.0.1:4317",
    trace: "on-first-retry",
  },
  webServer: [
    {
      command: "bun run ./e2e/mock-backend.ts",
      url: "http://127.0.0.1:8787/health",
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: "API_URL=http://127.0.0.1:8787 E2E_SUPPRESS_ABORT_LOGS=1 PORT=4317 bun run build && API_URL=http://127.0.0.1:8787 E2E_SUPPRESS_ABORT_LOGS=1 PORT=4317 bun run start",
      url: "http://127.0.0.1:4317",
      reuseExistingServer: false,
      timeout: 180_000,
    },
  ],
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
