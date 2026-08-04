import { expect, test, type Page } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const DEPLOYMENTS = "/testuser/agents/dep-slack-full-1/deployments";
const STATUS_ROUTE = /\/deployments\/[^/]+\/status$/;
const HISTORY_ROUTE = /\/deployment\/history(\?|$)/;

function deployingStatus() {
  return {
    value: "deploying",
    reason: "provisioning",
    details: "Pods are being provisioned",
  };
}

// A failed deployment whose failed_on names the cause. The failure banner is
// driven by the deployment's own failure state (value "error" + failed_on), not
// by scanning K8s events.
function failedStatus() {
  return {
    value: "error",
    reason: "failed",
    details: "Deployment failed: agent (Action required: Deployment stuck)",
    error_message: "FailedScheduling",
    failed_on: [
      {
        workload: "agent",
        component: "agent",
        phase: "failed",
        message: "Timed out waiting to become ready",
        title: "Action required: Deployment stuck",
        guidance:
          "This agent requests more CPU/memory than any node has available, so it can't be placed. Reduce its resources under Configure and redeploy.",
      },
    ],
  };
}

function routeJson(page: Page, re: RegExp, body: unknown) {
  return page.route(re, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) }),
  );
}

test.beforeEach(async () => {
  await resetMockBackend();
});

test("active deployment: Pause and Redeploy are available", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await page.getByRole("button", { name: "Deployment actions" }).click();
  await expect(page.getByRole("menuitem", { name: "Pause" })).toBeEnabled();
  await expect(page.getByRole("menuitem", { name: "Redeploy" })).toBeEnabled();
});

test("recovery actions stay enabled during a deploy (issue #1584)", async ({ page }) => {
  test.setTimeout(60_000);
  // A deploy in progress must not disable the escape actions, so the user is
  // never left without recourse while something stalls. Mid-deploy the escape
  // action is "Cancel deployment" (Pause is rejected by the server until the
  // deploy settles), alongside Redeploy and Restart.
  await routeJson(page, STATUS_ROUTE, deployingStatus());
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await page.getByRole("button", { name: "Deployment actions" }).click();
  await expect(page.getByRole("menuitem", { name: "Cancel deployment" })).toBeEnabled();
  await expect(page.getByRole("menuitem", { name: "Redeploy" })).toBeEnabled();
  await expect(page.getByRole("menuitem", { name: "Restart" })).toBeEnabled();
});

test("a failed deploy names the specific cause from its failure reason", async ({ page }) => {
  test.setTimeout(60_000);
  // The banner is driven by the deployment's own failure state (value "error"
  // + failed_on), which names the cause on its own.
  await routeJson(page, STATUS_ROUTE, failedStatus());
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("Action required: Deployment stuck")).toBeVisible();
  await expect(page.getByText(/requests more CPU\/memory than any node/)).toBeVisible();

  // The specific-cause variant offers a Copy fix prompt that seeds Claude Code.
  await page.getByRole("button", { name: "Copy fix prompt" }).click();
  await expect(page.getByText("Fix prompt copied")).toBeVisible();
});

test("a failed deploy with no earlier good version offers Pause as the primary action", async ({ page }) => {
  test.setTimeout(60_000);
  await routeJson(page, STATUS_ROUTE, failedStatus());
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("Action required: Deployment stuck")).toBeVisible();
  await expect(page.getByRole("link", { name: "Why deploys get stuck" })).toBeVisible();

  // With no earlier good version (the default fixture has one revision), the
  // primary action pauses the failed deploy directly.
  const stopped = page.waitForResponse(/\/deployments\/[^/]+\/stop$/);
  await page.getByRole("button", { name: "Pause", exact: true }).click();
  await stopped;
});

test("a failed deploy with an earlier good version offers a rollback", async ({ page }) => {
  test.setTimeout(60_000);
  await routeJson(page, STATUS_ROUTE, failedStatus());
  // History: the current (failed) revision plus an earlier healthy one to roll
  // back to. No `$` anchor: the request carries a `?deployment_id=` query.
  await routeJson(page, HISTORY_ROUTE, {
    deployments: [
      { id: "dep-slack-full-1", agent_name: "slack-config-full", revision: 3, build_id: "build-125", namespace: "ns", display_name: "Slack Full Bot", is_current: true, status: "failed", deployed_at: "2026-07-08T00:00:02Z", spec: {}, source: "github" },
      { id: "dep-slack-full-1", agent_name: "slack-config-full", revision: 2, build_id: "build-124", namespace: "ns", display_name: "Slack Full Bot", is_current: false, status: "healthy", deployed_at: "2026-07-08T00:00:01Z", spec: {}, source: "github" },
    ],
    count: 2,
  });
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  const rollback = page.getByRole("button", { name: "Roll back", exact: true });
  await expect(rollback).toBeVisible();
  await expect(page.getByRole("button", { name: "Pause", exact: true })).toBeVisible();
  await rollback.click();
  await expect(page).toHaveURL(/configure\?revision=2&build=build-124/);
});
