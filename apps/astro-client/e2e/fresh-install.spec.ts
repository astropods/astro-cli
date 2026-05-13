import { expect, test } from "@playwright/test";

const ACCOUNT = "testuser";
const AGENT_APP_TOKEN_ONLY = "code-reviewer";
const MOCK_BACKEND = "http://localhost:48787";

// Fresh installs add a new deployment to the mock backend's in-memory list,
// so reset between tests to avoid stale entries from prior runs.
test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

// Regression: if the deploy mutation only uses setQueryData (optimistic patch)
// and never invalidates, a brand-new deployment has no cache entry to patch and
// would silently vanish from the /agents list. This test catches that by asserting
// the new card appears immediately after redirect.
test("fresh install shows new deployment on agents list after redirect", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/deploy/${ACCOUNT}/${AGENT_APP_TOKEN_ONLY}`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByRole("button", { name: /slack/i }).click();
  await page.getByLabel("Slack App Token").fill("xapp-test-value");

  await Promise.all([
    page.waitForURL("**/agents*", { timeout: 20_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  // Dismiss the reveal overlay — during the overlay the matched card is shown as a
  // skeleton (to avoid stale-avatar flash), so the link won't be present until dismissed.
  // Use .click({ timeout }) which waits for the button to appear before clicking.
  await page.getByRole("button", { name: /close reveal/i }).click({ timeout: 10_000 }).catch(() => null);

  // The newly deployed agent should appear as a card on the agents list.
  const newAgentCard = page
    .locator(`a[href^="/${ACCOUNT}/agents/"]`)
    .filter({ hasText: AGENT_APP_TOKEN_ONLY })
    .first();
  await expect(newAgentCard).toBeVisible({ timeout: 10_000 });
});

// Regression: the optimistic setQueryData path for redeploys patches build_id on
// an existing entry. If that patch corrupts the entry (e.g. missing fields), the
// card could disappear or render incorrectly. This test deploys an agent that
// already exists in the mock and verifies the card stays visible after redirect.
test("re-deploying existing agent updates card on agents list", async ({ page }) => {
  test.setTimeout(60_000);

  // The "slack-config-full" agent already has a deployment in the mock. Visiting
  // its configure page and redeploying should keep the card on /agents.
  await page.goto(`/deploy/${ACCOUNT}/slack-config-full`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });

  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByRole("button", { name: /slack/i }).click();
  await expect(page.getByLabel("Slack Bot Token")).toBeVisible({ timeout: 10_000 });
  await page.getByLabel("Slack Bot Token").fill("xoxb-test-value");
  await page.getByLabel("Slack App Token").fill("xapp-test-value");

  await Promise.all([
    page.waitForURL("**/agents*", { timeout: 20_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  // Dismiss the reveal overlay — during the overlay the matched card is shown as a
  // skeleton (to avoid stale-avatar flash), so the link won't be present until dismissed.
  // Use .click({ timeout }) which waits for the button to appear before clicking.
  await page.getByRole("button", { name: /close reveal/i }).click({ timeout: 10_000 }).catch(() => null);

  const redeployedCard = page
    .locator(`a[href^="/${ACCOUNT}/agents/"]`)
    .filter({ hasText: "Slack Full Bot" })
    .first();
  await expect(redeployedCard).toBeVisible({ timeout: 10_000 });
});
