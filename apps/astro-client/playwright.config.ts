import { defineConfig, devices } from "@playwright/test";

const isCI = !!process.env.CI;
/** Only reuse manually-started servers (`E2E_REUSE_SERVER=1`). Default: Playwright owns lifecycle. */
const reuseExistingServer = process.env.E2E_REUSE_SERVER === "1";

export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,
  timeout: isCI ? 90_000 : 60_000,
  expect: { timeout: isCI ? 20_000 : 15_000 },
  reporter: isCI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: "http://127.0.0.1:44317",
    trace: "retain-on-failure",
    video: "retain-on-failure",
    actionTimeout: isCI ? 20_000 : 15_000,
    navigationTimeout: isCI ? 30_000 : 20_000,
  },
  webServer: [
    {
      command: "bun run ./e2e/mock-backend.ts",
      url: "http://127.0.0.1:48787/health",
      reuseExistingServer,
      timeout: isCI ? 60_000 : 30_000,
    },
    {
      command:
        "rm -rf build && VITE_API_URL= API_URL=http://127.0.0.1:48787 E2E_SUPPRESS_ABORT_LOGS=1 BIND_HOST=127.0.0.1 PORT=44317 bun run build && API_URL=http://127.0.0.1:48787 E2E_SUPPRESS_ABORT_LOGS=1 BIND_HOST=127.0.0.1 PORT=44317 bun run start",
      url: "http://127.0.0.1:44317",
      reuseExistingServer,
      timeout: isCI ? 300_000 : 180_000,
    },
  ],
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
