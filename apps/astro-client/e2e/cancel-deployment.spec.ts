import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// Regression: while a deployment is deploying, the history menu offers "Cancel
// deployment", and clicking it hits the cancel endpoint (the escape hatch for a
// stuck deploy).
const ACCOUNT = "testuser";
const ID = "dep-slack-full-1";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("offers Cancel deployment while deploying and calls the cancel endpoint", async ({ page }) => {
  test.setTimeout(60_000);
  await page.route("**/api/v1/deployments/*/status", (r) =>
    r.fulfill({ status: 200, contentType: "application/json",
      body: JSON.stringify({ value: "deploying", reason: "provisioning", details: "Pods are being provisioned" }) }));

  const cancelHits: string[] = [];
  page.on("request", (req) => {
    const p = new URL(req.url()).pathname;
    if (req.method() === "POST" && /\/deployments\/[^/]+\/cancel$/.test(p)) cancelHits.push(p);
  });

  await page.goto(`/${ACCOUNT}/agents/${ID}/deployments`, { waitUntil: "domcontentloaded" });
  const kebab = page.getByRole("button", { name: /deployment actions/i }).first();
  await expect(kebab).toBeVisible({ timeout: 20_000 });
  await kebab.click();
  const cancelItem = page.getByRole("menuitem", { name: /cancel deployment/i });
  await expect(cancelItem).toBeVisible();
  await cancelItem.click();
  await expect.poll(() => cancelHits.length).toBeGreaterThan(0);
});
