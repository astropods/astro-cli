import { expect, test, type Page } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// #1675: the deploy failure banner names the workload that actually failed, so a
// multi-container agent (agent vs collector sidecar) shows which one broke.
const DEPLOYMENTS = "/testuser/agents/dep-slack-full-1/deployments";
const STATUS_ROUTE = /\/deployments\/[^/]+\/status$/;

function imagePullFailure() {
  return {
    value: "error",
    reason: "failed",
    details: "Deployment failed: agent (Action required: Image pull failed)",
    error_message: "ImagePullBackOff",
    failed_on: [
      {
        workload: "slack-config-full-agent",
        component: "agent",
        phase: "failed",
        message: "Back-off pulling image",
        title: "Action required: Image pull failed",
        guidance:
          "The container keeps failing to pull its image. Push a new build with ast push, or trigger a rebuild if this agent deploys from GitHub, then redeploy.",
      },
    ],
  };
}
function routeJson(page: Page, re: RegExp, body: unknown) {
  return page.route(re, (r) => r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) }));
}

test.beforeEach(async () => { await resetMockBackend(); });

test("failure banner names the failing container", async ({ page }) => {
  test.setTimeout(60_000);
  await routeJson(page, STATUS_ROUTE, imagePullFailure());
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("Image pull failed for the agent container")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText(/Action required/)).toHaveCount(0);
});
