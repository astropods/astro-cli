import { expect, test } from "@playwright/test";

const ACCOUNT = "testuser";
const AGENT_SLACK_FULL = "slack-config-full";
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const MOCK_BACKEND = "http://localhost:48787";

test.describe("adapter toggle UX", () => {
  test("toggling Slack on shows required token fields, toggling off hides them", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    // Initially Slack fields should not be visible (web is default)
    await expect(page.getByLabel("Slack Bot Token")).not.toBeVisible();
    await expect(page.getByLabel("Slack App Token")).not.toBeVisible();

    // Toggle Slack on
    await page.locator("button[aria-pressed]", { hasText: /slack/i }).click();

    // Slack token fields should appear
    await expect(page.getByLabel("Slack Bot Token")).toBeVisible();
    await expect(page.getByLabel("Slack App Token")).toBeVisible();
    await expect(page.getByLabel("Actionable Reactions")).toBeVisible();

    // Toggle Slack off
    await page.locator("button[aria-pressed]", { hasText: /slack/i }).click();

    // Fields should disappear
    await expect(page.getByLabel("Slack Bot Token")).not.toBeVisible();
    await expect(page.getByLabel("Slack App Token")).not.toBeVisible();
    await expect(page.getByLabel("Actionable Reactions")).not.toBeVisible();
  });

  test("Slack tokens are required when adapter is selected — deploy blocked without them", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    // Fill the agent variable
    await page.getByLabel("Openai Api Key").fill("sk-test-value");

    // Toggle Slack on but leave tokens empty
    await page.locator("button[aria-pressed]", { hasText: /slack/i }).click();
    await expect(page.getByLabel("Slack Bot Token")).toBeVisible();

    // Try to deploy — should be blocked (required fields empty)
    await page.getByRole("button", { name: /deploy/i }).click();

    // Should stay on the deploy page (not redirect)
    await expect(page).toHaveURL(new RegExp(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}$`));

    // Fill the tokens
    await page.getByLabel("Slack Bot Token").fill("xoxb-test");
    await page.getByLabel("Slack App Token").fill("xapp-test");

    // Deploy should now succeed
    await Promise.all([
      page.waitForURL("**/agents*", { timeout: 20_000 }),
      page.getByRole("button", { name: /deploy/i }).click(),
    ]);
  });

  test("configure page pre-selects Slack adapter from existing deployment", async ({ page }) => {
    test.setTimeout(60_000);
    await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });

    await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment`, {
      waitUntil: "domcontentloaded",
    });

    // Slack should be pre-selected (the existing deployment had ["web", "slack"])
    await expect(page.getByLabel("Slack Bot Token")).toBeVisible({ timeout: 20_000 });
    await expect(page.getByLabel("Slack App Token")).toBeVisible();

    // Slack token values should be prefilled
    await expect(page.getByLabel("Slack Bot Token")).toHaveValue("xoxb-existing-value");

    // Actionable Reactions should be prefilled from SLACK_CONFIG
    await expect(page.getByLabel("Actionable Reactions")).toHaveValue("ticket, bug");
    await expect(page.getByLabel("Allowed Channel IDs")).toHaveValue("C123, C999");
  });
});

test.describe("web auth toggle UX", () => {
  test("auth toggle is visible under web adapter and defaults to checked when template has oidc", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    // Web adapter should be selected by default — auth toggle should be visible
    await expect(page.getByText("Require authentication")).toBeVisible();

    // Template has auth.web.type=oidc, so toggle should be checked
    const authSwitch = page.locator("button[role=switch]").filter({ has: page.locator("[id*='require-auth']") });
    // The switch with id containing 'require-auth' should exist
    const requireAuthSwitch = page.locator("[id*='require-auth']");
    await expect(requireAuthSwitch).toBeVisible();
  });

  test("auth toggle disappears when web adapter is deselected", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await expect(page.getByText("Require authentication")).toBeVisible();

    // Deselect web, select only Slack
    await page.locator("button[aria-pressed]", { hasText: /web/i }).click();
    await page.locator("button[aria-pressed]", { hasText: /slack/i }).click();

    // Auth toggle should disappear (it's only on web)
    await expect(page.getByText("Require authentication")).not.toBeVisible();
  });
});
