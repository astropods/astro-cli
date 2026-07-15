import { expect, test, type Page } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const DEPLOYMENTS = "/testuser/agents/dep-slack-full-1/deployments";
const STATUS_ROUTE = /\/deployments\/[^/]+\/status$/;
const EVENTS_ROUTE = /\/deployments\/[^/]+\/events$/;
const HISTORY_ROUTE = /\/deployment\/history(\?|$)/;

// A deploying status whose status_changed_at is well in the past, so the
// real-age stuck backstop fires immediately. Measuring from a server timestamp
// (not page load) is the point of the mechanism, so no clock manipulation is
// needed to exercise it.
const LONG_AGO = "2020-01-01T00:00:00Z";

function deployingStatus(statusChangedAt?: string) {
  return {
    value: "deploying",
    reason: "provisioning",
    details: "Pods are being provisioned",
    ...(statusChangedAt ? { status_changed_at: statusChangedAt } : {}),
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
  // never left without recourse while something stalls.
  await routeJson(page, STATUS_ROUTE, deployingStatus());
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await page.getByRole("button", { name: "Deployment actions" }).click();
  await expect(page.getByRole("menuitem", { name: "Pause" })).toBeEnabled();
  await expect(page.getByRole("menuitem", { name: "Redeploy" })).toBeEnabled();
  await expect(page.getByRole("menuitem", { name: "Restart" })).toBeEnabled();
});

test("a stuck deploy names the specific cause from a humanized event", async ({ page }) => {
  test.setTimeout(60_000);
  // No status_changed_at means the real-age backstop never fires, so this proves
  // the banner is event-driven: a stuck-severity event surfaces the banner and
  // names the cause on its own.
  await routeJson(page, STATUS_ROUTE, deployingStatus());
  await routeJson(page, EVENTS_ROUTE, {
    events: [
      {
        type: "Warning",
        reason: "FailedScheduling",
        message: "0/3 nodes are available: insufficient cpu",
        object_kind: "Pod",
        object_name: "agent-xyz",
        count: 1,
        first_timestamp: "2026-07-08T00:00:00Z",
        last_timestamp: "2026-07-08T00:05:00Z",
        title: "Action required: Deployment stuck",
        guidance: "This agent requests more CPU/memory than any node has available, so it can't be placed. Reduce its resources under Configure and redeploy.",
        severity: "stuck",
      },
    ],
  });
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("Action required: Deployment stuck")).toBeVisible();
  await expect(page.getByText(/requests more CPU\/memory than any node/)).toBeVisible();

  // The specific-cause variant offers a Copy fix prompt that seeds Claude Code.
  await page.getByRole("button", { name: "Copy fix prompt" }).click();
  await expect(page.getByText("Fix prompt copied")).toBeVisible();
});

test("a deploy stuck past the timeout shows the banner (real deploy age)", async ({ page }) => {
  test.setTimeout(60_000);
  // No stuck event, but the deploy has been in "deploying" far longer than the
  // threshold (status_changed_at long ago), so the defensive age backstop fires.
  await routeJson(page, STATUS_ROUTE, deployingStatus(LONG_AGO));
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("This deploy is stuck")).toBeVisible();
  await expect(page.getByRole("link", { name: "Why deploys get stuck" })).toBeVisible();

  // With no earlier good version (the default fixture has one revision), the
  // primary action pauses the stuck deploy directly.
  const stopped = page.waitForResponse(/\/deployments\/[^/]+\/stop$/);
  await page.getByRole("button", { name: "Pause", exact: true }).click();
  await stopped;
});

test("a stuck deploy with an earlier good version offers a rollback", async ({ page }) => {
  test.setTimeout(60_000);
  await routeJson(page, STATUS_ROUTE, deployingStatus(LONG_AGO));
  // History: the current (stuck) revision plus an earlier healthy one to roll
  // back to. No `$` anchor: the request carries a `?deployment_id=` query.
  await routeJson(page, HISTORY_ROUTE, {
    deployments: [
      { id: "dep-slack-full-1", agent_name: "slack-config-full", revision: 3, build_id: "build-125", namespace: "ns", display_name: "Slack Full Bot", is_current: true, status: "deploying", deployed_at: "2026-07-08T00:00:02Z", spec: {}, source: "github" },
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
