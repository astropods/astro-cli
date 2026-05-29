import { test, expect } from "./fixtures";
import { existsSync, readFileSync } from "fs";
import { CLI_STATE_FILE } from "./cli-state";
import { envConfig } from "./env";

const username = envConfig.username;

test(
  "hello-astro — deployment becomes active",
  { timeout: 900000 }, // 15 min: 14 min poll + headroom
  async ({ page }) => {
    if (!existsSync(CLI_STATE_FILE)) {
      throw new Error("CLI state file not found — app.deploy spec must run first");
    }
    const { deploymentSlug } = JSON.parse(readFileSync(CLI_STATE_FILE, "utf-8"));
    expect(deploymentSlug, "deploymentSlug not captured by app.deploy spec").toBeTruthy();

    await page.goto(`/${username}/agents/${deploymentSlug}`, { waitUntil: "load" });

    const statusToggle = page.getByTestId("agent-status-toggle");
    await expect(
      statusToggle.getByText("Active", { exact: true }),
      "Hello Astro deployment did not become Active within 14 minutes",
    ).toBeVisible({ timeout: 840000 });
  },
);
