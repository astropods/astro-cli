import { defineConfig } from "@playwright/test";


export default defineConfig({
  testDir: "./tests/smoke",
  dotEnvPath: ".env.local",
  timeout: 60000,
  retries: 1,
  workers: 2,
  use: {
    baseURL: process.env.ASTRO_TEST_HOST ?? "https://astropods.com",
    headless: true,
    actionTimeout: 10000,
    navigationTimeout: 30000,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  projects: [
    // 1. Login once, save auth state. If this fails, all app-site tests are skipped.
    {
      name: "setup",
      testMatch: "**/auth.setup.ts",
      retries: 0,
      use: { browserName: "chromium" },
    },

    // 2. Marketing site — no auth required, runs in parallel with setup.
    {
      name: "marketing-site",
      testMatch: "**/public.spec.ts",
      use: { browserName: "chromium" },
    },

    // 3. Blueprint pages — no auth required, runs in all environments.
    {
      name: "blueprints",
      testMatch: "**/public.blueprint.spec.ts",
      use: { browserName: "chromium" },
    },

    // 3. Auth smoke test — just needs a valid session. Runs before CLI and app tests.
    {
      name: "auth",
      testMatch: "**/auth.spec.ts",
      dependencies: ["setup"],
      use: {
        browserName: "chromium",
        storageState: "playwright/.auth/user.json",
      },
    },

    // 4. CLI — install + device-flow login. Runs after auth is confirmed.
    //    Teardown runs after cli AND app.deploy have both finished.
    {
      name: "cli",
      testMatch: "**/cli.spec.ts",
      dependencies: ["auth"],
      teardown: "cli-teardown",
      use: {
        browserName: "chromium",
        storageState: "playwright/.auth/user.json",
      },
    },

    // 4a. CLI teardown — archives hello-astro and removes sandbox. Managed by Playwright,
    //     do not run directly.
    {
      name: "cli-teardown",
      testMatch: "**/cli.teardown.ts",
      use: { browserName: "chromium" },
    },

    // 5. Deploy — verifies hello-astro blueprint page and deploy flow after push.
    {
      name: "app.deploy",
      testMatch: "**/app.deploy.spec.ts",
      dependencies: ["cli"],
      use: {
        browserName: "chromium",
        storageState: "playwright/.auth/user.json",
      },
    },

    // 6. CLI post-deploy — verifies the deployed agent appears in ast agent list.
    {
      name: "cli.post-deploy",
      testMatch: "**/cli.post-deploy.spec.ts",
      dependencies: ["app.deploy"],
      use: { browserName: "chromium" },
    },

    // 7. Post-deploy — waits for hello-astro deployment to become Active.
    {
      name: "app.post-deploy",
      testMatch: "**/app.post-deploy.spec.ts",
      dependencies: ["app.deploy"],
      use: {
        browserName: "chromium",
        storageState: "playwright/.auth/user.json",
      },
    },

    // 8. Chat — clicks the agent card and verifies the chat echo response.
    // TODO: re-enable once chat test is stable
    // {
    //   name: "app.chat",
    //   testMatch: "**/app.chat.spec.ts",
    //   dependencies: ["app.post-deploy"],
    //   use: {
    //     browserName: "chromium",
    //     storageState: "playwright/.auth/user.json",
    //   },
    // },

    // 7. Secrets & variables — creates variables then verifies they auto-fill on the deploy page.
    {
      name: "app.secrets",
      testMatch: "**/app.secrets.spec.ts",
      dependencies: ["auth"],
      use: {
        browserName: "chromium",
        storageState: "playwright/.auth/user.json",
      },
    },
  ],
});
