import { expect, test } from "@playwright/test";

const ACCOUNT = "testuser";
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const DEPLOYMENT_SLACK_OVERLAP_ID = "dep-slack-overlap-1";
const BUILD_UPGRADE_LABEL = "build-123 \u2192 build-124";
const MOCK_BACKEND = "http://localhost:48787";

// The mock backend is stateful (deploys update build_id), so reset between tests.
test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

// Ensures the My Agents card advertises version drift when latest published build
// is newer than the deployment's current build. The card shows a minimal "update"
// badge rather than the full build ID transition.
test("my agents card shows new build badge for out-of-date deployment", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  const staleCard = page.locator(`a[href="/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}"]`);
  await expect(staleCard).toBeVisible({ timeout: 20_000 });
  await expect(staleCard.getByText("update", { exact: true })).toBeVisible();
});

// Mirrors card-level drift signal on the deployment detail header so users can
// discover newer builds from either the list view or detail view.
test("deployment detail header shows new build badge when newer build exists", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByRole("heading", { name: "Slack Full Bot" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText(BUILD_UPGRADE_LABEL)).toBeVisible();
});

// Guards against false-positive badge rendering: when deployed build matches the
// latest published build, no "New build" indicator should appear.
test("up-to-date deployment does not show new build badge", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_OVERLAP_ID}`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByRole("heading", { name: "Slack Overlap Bot" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("New build")).toHaveCount(0);
});

// The current build_id should always be visible on the detail page as
// informational context, regardless of whether a newer build exists.
test("detail page always shows current build id badge", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_OVERLAP_ID}`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByRole("heading", { name: "Slack Overlap Bot" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("build-123")).toBeVisible();
});

// Build-only upgrade flow: with no dirty config edits, configure page should still
// surface a Redeploy action and allow submitting current config against latest build.
test("configure page shows build-only redeploy button and redeploys without edits", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByText("New build available")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText(BUILD_UPGRADE_LABEL)).toBeVisible();
  await expect(page.getByRole("button", { name: /^redeploy$/i })).toBeVisible();
  await expect(page.getByRole("button", { name: /^cancel$/i })).toHaveCount(0);

  const redeployRequest = page.waitForRequest(
    (request) => request.method() === "POST" && request.url().includes("/api/v1/deploy"),
  );

  await Promise.all([
    redeployRequest,
    page.waitForURL(`**/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}`, { timeout: 20_000 }),
    page.getByRole("button", { name: /^redeploy$/i }).click(),
  ]);

  const payload = (await redeployRequest).postDataJSON() as {
    variables?: Record<string, { value?: string }>;
  };
  expect(payload.variables?.OPENAI_API_KEY?.value).toBe("sk-existing-value");
  expect(payload.variables?.SLACK_BOT_TOKEN?.value).toBe("xoxb-existing-value");
});

// After a successful build-only redeploy, the optimistic cache update should
// clear the badge immediately on the detail page without waiting for a server round-trip.
test("new build badge clears after successful redeploy", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByText("New build available")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("button", { name: /^redeploy$/i })).toBeVisible();

  await Promise.all([
    page.waitForURL(`**/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}`, { timeout: 20_000 }),
    page.getByRole("button", { name: /^redeploy$/i }).click(),
  ]);

  await expect(page.getByRole("heading", { name: "Slack Full Bot" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("New build")).toHaveCount(0);
});

// With a newer build available, the bar starts as build-only ("Redeploy"). Editing a
// config field should transition it to dirty mode ("Save & Redeploy") with a cancel button.
test("editing config with newer build switches bar from redeploy to save and redeploy", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByText("New build available")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("button", { name: /^redeploy$/i })).toBeVisible();
  await expect(page.getByRole("button", { name: /^cancel$/i })).toHaveCount(0);

  await page.getByLabel("Slack Bot Token").fill("xoxb-changed-value");

  await expect(page.getByText("New build available")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /save\s*&\s*redeploy/i })).toBeVisible();
  await expect(page.getByRole("button", { name: /^cancel$/i })).toBeVisible();

  // Clicking Cancel resets the form, restoring the build-only bar
  await page.getByRole("button", { name: /^cancel$/i }).click();

  await expect(page.getByText("New build available")).toBeVisible();
  await expect(page.getByRole("button", { name: /^redeploy$/i })).toBeVisible();
  await expect(page.getByRole("button", { name: /^cancel$/i })).toHaveCount(0);
});

// When the deploy API rejects a redeploy, the user should stay on the configure
// page with the error displayed and the action bar still available for retry.
// Editing a field makes the form dirty, so the bar shows change count + "Save & Redeploy"
// rather than the build-only badge.
test("failed redeploy keeps user on configure page with action bar", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByText("New build available")).toBeVisible({ timeout: 20_000 });

  // Change the bot token to the sentinel that triggers a 400 from the mock backend
  await page.getByLabel("Slack Bot Token").fill("xoxb-server-reject");

  await page.getByRole("button", { name: /save\s*&\s*redeploy/i }).click();

  await expect(page).toHaveURL(
    new RegExp(`/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}/configure/deployment$`),
  );
  await expect(page.getByText("Deployment failed")).toBeVisible();
  await expect(page.getByRole("button", { name: /save\s*&\s*redeploy/i })).toBeVisible();
});
