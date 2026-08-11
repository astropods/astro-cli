import { expect, test, type Page } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// Regression for #1876: after Redeploy from Configure the Deployments tab lands
// while the new deployment has no pods yet. It must show a deploying state, not
// a bare "No active pods" (which read as a blank/broken page).

// dep-slack-overlap-1 has no workloads in the fixture, matching the just-after-
// redeploy state where pods haven't been created yet.
const DEPLOYMENTS = "/testuser/agents/dep-slack-overlap-1/deployments";
const STATUS_ROUTE = /\/deployments\/[^/]+\/status$/;

function deployingStatus() {
  return { value: "deploying", reason: "provisioning", details: "Pods are being provisioned" };
}
function routeJson(page: Page, re: RegExp, body: unknown) {
  return page.route(re, (r) => r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) }));
}

test.beforeEach(async () => { await resetMockBackend(); });

test("deployments tab shows a deploying state (not blank) while pods come up", async ({ page }) => {
  test.setTimeout(60_000);
  await routeJson(page, STATUS_ROUTE, deployingStatus());
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("Deploying your agent…")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("No active pods")).toHaveCount(0);
});
