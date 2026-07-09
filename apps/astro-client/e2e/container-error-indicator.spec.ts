import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

const DEPLOYMENTS = "/testuser/agents/dep-slack-full-1/deployments";
const LOGS_ROUTE = /\/deployments\/[^/]+\/logs\?/;

test.beforeEach(async () => {
  await resetMockBackend();
});

test("no error indicator when container logs are clean", async ({ page }) => {
  test.setTimeout(60_000);
  // Default mock backend returns no error logs.
  await page.route(LOGS_ROUTE, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "[]" }),
  );
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("agent", { exact: true })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByLabel("Errors found in logs")).toHaveCount(0);
});

test("shows an error indicator when a container has error logs", async ({ page }) => {
  test.setTimeout(60_000);
  // Force an error-level log so the healthy agent pod surfaces the indicator.
  await page.route(LOGS_ROUTE, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([
        { level: "error", message: "boom: request failed", timestamp: "2026-07-07T00:00:00Z" },
      ]),
    }),
  );
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await expect(page.getByText("agent", { exact: true })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByLabel("Errors found in logs")).toBeVisible({ timeout: 10_000 });
});

test("clicking a pod with errors opens the detail panel with an error banner", async ({ page }) => {
  test.setTimeout(60_000);
  await page.route(LOGS_ROUTE, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify([
        { level: "error", message: "boom: request failed", timestamp: "2026-07-07T00:00:00Z" },
      ]),
    }),
  );
  await page.goto(DEPLOYMENTS, { waitUntil: "domcontentloaded" });

  await page.getByText("agent", { exact: true }).click();

  // The panel surfaces the error as a banner instead of an empty General tab.
  // (The message also appears in the auto-opened Logs tab, hence .first().)
  await expect(page.getByText("Errors in logs")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByText("boom: request failed").first()).toBeVisible();
});
