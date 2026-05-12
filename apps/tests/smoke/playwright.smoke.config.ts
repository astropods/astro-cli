import path from "path";
import { defineConfig } from "@playwright/test";
import { envConfig } from "./env";

const isPreview = process.env.ASTRO_ENV === "preview";
const isDev = process.env.ASTRO_ENV === "dev";
const authFile = path.join(import.meta.dirname, "playwright/.auth/user.json");

export default defineConfig({
  testDir: ".",
  timeout: 60000,
  retries: 1,
  workers: 2,
  use: {
    baseURL: process.env.ASTRO_TEST_HOST || envConfig.appBaseUrl,
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

    // 2. Marketing site — no auth required, runs in parallel with setup. Skipped on dev.
    ...(!isDev
      ? [
          {
            name: "marketing-site",
            testMatch: "**/public.spec.ts",
            use: { browserName: "chromium" },
          },

          // 3. Blueprint pages — no auth required, runs in all environments except dev.
          {
            name: "blueprints",
            testMatch: "**/public.blueprint.spec.ts",
            use: { browserName: "chromium" },
          },
        ]
      : []),

    // 4. Auth smoke test — just needs a valid session. Runs before CLI and app tests.
    {
      name: "auth",
      testMatch: "**/auth.spec.ts",
      dependencies: ["setup"],
      use: {
        browserName: "chromium",
        storageState: authFile,
      },
    },

    // CLI + downstream projects — skipped on preview because the registry is not
    // accessible from GitHub Actions in that environment.
    ...(!isPreview
      ? [
          // 5. CLI — install + device-flow login. Runs after auth is confirmed.
          //    Teardown runs after cli AND app.deploy have both finished.
          {
            name: "cli",
            testMatch: "**/cli.spec.ts",
            retries: 0, // serial group shares fakeHome — retrying from scratch breaks state
            dependencies: ["auth"],
            teardown: "cli-teardown",
            use: {
              browserName: "chromium",
              storageState: authFile,
            },
          },

          // 5a. CLI teardown — archives hello-astro and removes sandbox. Managed by Playwright,
          //     do not run directly.
          {
            name: "cli-teardown",
            testMatch: "**/cli.teardown.ts",
            use: { browserName: "chromium" },
          },

          // 6. Deploy — verifies hello-astro blueprint page and deploy flow after push.
          {
            name: "app.deploy",
            testMatch: "**/app.deploy.spec.ts",
            dependencies: ["cli"],
            use: {
              browserName: "chromium",
              storageState: authFile,
            },
          },

          // 7. CLI post-deploy — verifies the deployed agent appears in ast agent list.
          {
            name: "cli.post-deploy",
            testMatch: "**/cli.post-deploy.spec.ts",
            dependencies: ["app.deploy"],
            use: { browserName: "chromium" },
          },

          // 8. Post-deploy — waits for hello-astro deployment to become Active.
          {
            name: "app.post-deploy",
            testMatch: "**/app.post-deploy.spec.ts",
            dependencies: ["app.deploy"],
            use: {
              browserName: "chromium",
              storageState: authFile,
            },
          },
        ]
      : []),

    // 9. Chat — clicks the agent card and verifies the chat echo response.
    // TODO: re-enable once chat test is stable
    // {
    //   name: "app.chat",
    //   testMatch: "**/app.chat.spec.ts",
    //   dependencies: ["app.post-deploy"],
    //   use: {
    //     browserName: "chromium",
    //     storageState: authFile,
    //   },
    // },

    // 10. Secrets & variables — creates variables then verifies they auto-fill on the deploy page.
    {
      name: "app.secrets",
      testMatch: "**/app.secrets.spec.ts",
      dependencies: ["auth"],
      use: {
        browserName: "chromium",
        storageState: authFile,
      },
    },
  ],
});
