import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const ACCOUNT = "testuser";
const AGENT_SLACK_FULL = "slack-config-full";
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";

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

    // Fill the agent variable only if vault didn't auto-fill it
    const apiKeyInput = page.getByLabel("Openai Api Key");
    if (await apiKeyInput.isVisible() && !(await apiKeyInput.inputValue())) {
      await apiKeyInput.fill("sk-test-value");
    }

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
    await resetMockBackend();

    await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure`, {
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

test.describe("auth grants UX", () => {
  test("grants editor renders under web adapter with a 'Specific user…' option", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    // Web adapter is selected by default — grants editor should be visible.
    await expect(page.getByText("Grant access")).toBeVisible();

    // Web's add-grant menu offers a per-user grant.
    await page.getByRole("button", { name: /add access/i }).click();
    await expect(page.getByRole("menuitem", { name: /specific user/i })).toBeVisible();
  });

  test("slack-only selection shows grants editor with a 'Specific user…' option", async ({ page }) => {
    test.setTimeout(60_000);
    await page.goto(`/deploy/${ACCOUNT}/${AGENT_SLACK_FULL}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

    await expect(page.getByText("Grant access")).toBeVisible();

    // Deselect web, select only Slack.
    await page.locator("button[aria-pressed]", { hasText: /web/i }).click();
    await page.locator("button[aria-pressed]", { hasText: /slack/i }).click();

    // Grants editor still visible (slack has its own).
    await expect(page.getByText("Grant access")).toBeVisible();

    // Fresh-deploy default for Slack is an "anyone" grant. That grant
    // subsumes everything else, so the add-grant menu is intentionally
    // hidden — remove the default first to reveal it.
    await expect(page.getByText(/^Anyone$/)).toBeVisible();
    await page.getByRole("button", { name: /remove grant/i }).click();

    // Slack's add-grant menu offers per-user grants too — slack identities
    // resolve to the linked WorkOS user server-side, so user grants apply.
    await page.getByRole("button", { name: /add access/i }).click();
    await expect(page.getByRole("menuitem", { name: /anyone/i })).toBeVisible();
    await expect(page.getByRole("menuitem", { name: /specific user/i })).toBeVisible();
  });
});
