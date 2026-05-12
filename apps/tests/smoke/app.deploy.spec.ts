import { test, expect } from "./fixtures";
import { existsSync, readFileSync, writeFileSync } from "fs";
import { join } from "path";
import { CLI_STATE_FILE } from "./cli-state";
import { envConfig } from "./env";

const username = envConfig.username;
const BLUEPRINT_URL = `/${username}/hello-astro`;

test.describe("blueprint deploy", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(BLUEPRINT_URL, { waitUntil: "load" });
  });

  test("shows agent name and description", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "hello-astro" })).toBeVisible();
  });

  test("deploy button is present", async ({ page }) => {
    await expect(page.getByRole("link", { name: /deploy this agent/i })).toBeVisible();
  });
});

// Serial so the slug captured in the deploy test is available to the agent list check.
let deploymentSlug: string;

test.describe("hello-astro deploy flow", () => {
  test.describe.configure({ mode: "serial" });

  test("deploy button opens deploy flow for authenticated user", async ({ page }) => {
    await page.goto(BLUEPRINT_URL, { waitUntil: "load" });
    await page.getByRole("link", { name: /deploy this agent/i }).last().click();
    await page.waitForURL(new RegExp(`/deploy/${username}/hello-astro`), { timeout: 15000 });
    await expect(page).not.toHaveURL(envConfig.loginUrlPattern);
    await page.getByRole("button", { name: /^deploy$/i }).click();

    // Confirm the deploying popup appears
    await expect(page.getByRole("heading", { name: /hello astro is deploying!/i })).toBeVisible({ timeout: 15000 });
    await expect(page.locator(".holo-card")).toBeVisible();

    // Navigate to the deployment and capture the slug
    await page.getByRole("button", { name: /view deployment/i }).click();
    await page.waitForURL(new RegExp(`/${username}/agents/[a-z0-9-]+`), { timeout: 15000 });
    const slugMatch = page.url().match(new RegExp(`/${username}/agents/([a-z0-9-]+)`));
    expect(slugMatch, `URL ${page.url()} did not match expected deployment path /${username}/agents/<slug>`).toBeTruthy();
    deploymentSlug = slugMatch![1];
    console.log("deploymentSlug:", deploymentSlug);

    // Persist slug so cli.teardown can verify it in ast agent list
    if (existsSync(CLI_STATE_FILE)) {
      const state = JSON.parse(readFileSync(CLI_STATE_FILE, "utf-8"));
      writeFileSync(CLI_STATE_FILE, JSON.stringify({ ...state, deploymentSlug }));
    }
  });
});
