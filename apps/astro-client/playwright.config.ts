import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:44317",
    trace: "on-first-retry",
  },
  webServer: [
    {
      command: "bun run ./e2e/mock-backend.ts",
      url: "http://127.0.0.1:48787/health",
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: "API_URL=http://127.0.0.1:48787 E2E_SUPPRESS_ABORT_LOGS=1 PORT=44317 bun run build && API_URL=http://127.0.0.1:48787 E2E_SUPPRESS_ABORT_LOGS=1 PORT=44317 bun run start",
      url: "http://127.0.0.1:44317",
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
