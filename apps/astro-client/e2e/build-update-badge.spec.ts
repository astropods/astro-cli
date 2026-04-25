import { expect, test } from "@playwright/test";

const ACCOUNT = "testuser";
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const DEPLOYMENT_SLACK_OVERLAP_ID = "dep-slack-overlap-1";
const DEPLOYMENT_XACCT_UPGRADE_ID = "dep-xacct-upgrade-1";
const DEPLOYMENT_XACCT_COLLISION_ID = "dep-xacct-collision-1";
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

  const staleCard = page.locator(`a[href^="/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}"]`);
  await expect(staleCard).toBeVisible({ timeout: 20_000 });
  await expect(staleCard.getByText("Update available", { exact: true })).toBeVisible();
});

// Cross-account upgrade signal: a deployment whose source_account differs
// from the viewer's account must surface the upgrade badge using the
// source account's blueprint listing. The personal account does NOT
// publish a blueprint with this name, so a name-only lookup against the
// viewer's account would have left the badge silent (the pre-fix
// behavior reproduced in production with the issueator agent).
test("cross-account deployment shows update badge from source account blueprint", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  const upgradeCard = page.locator(`a[href^="/${ACCOUNT}/agents/${DEPLOYMENT_XACCT_UPGRADE_ID}"]`);
  await expect(upgradeCard).toBeVisible({ timeout: 20_000 });
  await expect(upgradeCard.getByText("Update available", { exact: true })).toBeVisible();
});

// Cross-account name-collision suppression: a deployment whose
// source_account is up-to-date in its publisher's blueprint must NOT
// show the upgrade badge, even though the viewer's personal account
// publishes a same-named but lineage-unrelated blueprint with a newer
// build. Pre-fix the dashboard reducer matched by name only against the
// viewer's account and produced a misleading badge whose Redeploy 404'd
// on the server because the deployment's build_id was not in that
// lineage.
//
// The slack-full card is asserted in parallel as a regression guard so a
// universal "no badges anywhere" failure can't masquerade as a pass.
test("cross-account collision deployment does not show update badge from personal-account collision", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  const collisionCard = page.locator(`a[href^="/${ACCOUNT}/agents/${DEPLOYMENT_XACCT_COLLISION_ID}"]`);
  const validUpgradeCard = page.locator(`a[href^="/${ACCOUNT}/agents/${DEPLOYMENT_SLACK_FULL_ID}"]`);
  await expect(collisionCard).toBeVisible({ timeout: 20_000 });
  await expect(validUpgradeCard.getByText("Update available", { exact: true })).toBeVisible();

  await expect(collisionCard.getByText("Update available", { exact: true })).toHaveCount(0);
});

// Same collision scenario on the detail page: the Redeploy banner is the
// primary path into the false-positive upgrade and must not render when
// the source account's blueprint shows no upgrade — even when the
// viewer's personal account has a newer same-named build.
test("cross-account collision deployment does not show redeploy banner on detail page", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_XACCT_COLLISION_ID}`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByRole("heading", { name: "Cross-Account Collision Bot" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("button", { name: /Redeploy →/ })).toHaveCount(0);
});

// Cross-account upgrade signal on the detail page: the Redeploy banner
// must render for a legitimate cross-account upgrade so users can act on
// it from the deployment detail view, not just the dashboard.
test("cross-account deployment shows redeploy banner on detail page", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(`/${ACCOUNT}/agents/${DEPLOYMENT_XACCT_UPGRADE_ID}`, {
    waitUntil: "domcontentloaded",
  });

  await expect(page.getByRole("heading", { name: "Cross-Account Upgrade Bot" })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("button", { name: /Redeploy →/ })).toBeVisible();
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
  await expect(page.getByText("Validation failed")).toBeVisible();
  await expect(page.getByRole("button", { name: /save\s*&\s*redeploy/i })).toBeVisible();
});
