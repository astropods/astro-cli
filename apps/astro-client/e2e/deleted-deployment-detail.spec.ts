import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// Regression for #1340: navigating to a deleted/unknown deployment (e.g. from a
// stale Insights link) must show a not-found state, not crash a child tab on a
// null context.
const ACCOUNT = "testuser";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("deleted deployment detail shows a not-found state, not a crash", async ({ page }) => {
  const pageErrors: string[] = [];
  page.on("pageerror", (e) => pageErrors.push(String(e)));

  await page.goto(`/${ACCOUNT}/agents/dep-does-not-exist/monitor`, { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("heading", { name: /deployment not found/i })).toBeVisible({ timeout: 20_000 });
  await expect(page.getByRole("link", { name: /back to agents/i })).toBeVisible();
  expect(pageErrors.join("\n")).not.toMatch(/destructure|Cannot read/i);
});
