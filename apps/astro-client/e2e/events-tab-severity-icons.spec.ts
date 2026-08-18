import { expect, test, type Page } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// #1749: the pod Events tab marks each event with a labelled severity icon, so
// severity reads from shape + label (accessible), not color alone.
const DEPLOYMENTS = "/testuser/agents/dep-slack-full-1/deployments";
const EVENTS_ROUTE = /\/deployments\/[^/]+\/events/;

function events() {
  return {
    events: [
      {
        type: "Warning", reason: "BackOff", message: "Back-off restarting failed container",
        object_kind: "Pod", object_name: "slack-config-full-agent-abc", count: 6,
        first_timestamp: "2026-08-01T00:00:00Z", last_timestamp: "2026-08-01T00:05:00Z",
        title: "Action required: Container crash looping",
        guidance: "The container keeps starting and exiting. Check the pod logs for the cause.",
        severity: "stuck",
      },
      {
        type: "Warning", reason: "Unhealthy", message: "Readiness probe failed: HTTP 503",
        object_kind: "Pod", object_name: "slack-config-full-agent-abc", count: 3,
        first_timestamp: "2026-08-01T00:01:00Z", last_timestamp: "2026-08-01T00:04:00Z",
      },
      {
        type: "Normal", reason: "Pulled", message: "Successfully pulled image",
        object_kind: "Pod", object_name: "slack-config-full-agent-abc", count: 1,
        first_timestamp: "2026-08-01T00:00:30Z", last_timestamp: "2026-08-01T00:00:30Z",
      },
    ],
  };
}
function routeJson(page: Page, re: RegExp, body: unknown) {
  return page.route(re, (r) => r.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) }));
}

test.beforeEach(async () => { await resetMockBackend(); });

test("Events tab marks each event with a labelled severity icon", async ({ page }) => {
  test.setTimeout(60_000);
  await routeJson(page, EVENTS_ROUTE, events());
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await page.getByText("agent", { exact: true }).first().click();
  await page.getByRole("button", { name: "Events" }).click();

  await expect(page.getByText("Action required: Container crash looping")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByLabel("Needs attention")).toBeVisible();
  await expect(page.getByLabel("Warning").first()).toBeVisible();
  await expect(page.getByLabel("Info")).toBeVisible();
});
