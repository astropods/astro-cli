import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";


test.beforeEach(async () => {
  await resetMockBackend();
});

test("dashboard shows deployed agents", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("heading", { level: 1, name: "Agents" })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("Slack Full Bot")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("Cross Account Agent")).toBeVisible({ timeout: 10_000 });
});

test("dashboard search filter narrows visible agents", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  await expect(page.getByText("Slack Full Bot")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("Slack Overlap Bot")).toBeVisible({ timeout: 10_000 });

  await page.getByPlaceholder("Search agents...").fill("full");

  await expect(page.getByText("Slack Full Bot")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Slack Overlap Bot")).not.toBeVisible();
});

test("dashboard account scope persists while search resets after navigation", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  const search = page.getByPlaceholder("Search agents...");
  await expect(search).toBeVisible({ timeout: 10_000 });
  await search.fill("full");
  const scopeSwitcher = page.getByRole("button", { name: "Switch account" });
  await scopeSwitcher.click();
  await page.getByRole("menuitem").filter({ hasText: "Test Org" }).click();
  await expect(scopeSwitcher).toContainText("Test Org");

  await page.getByRole("link", { name: "Blueprints", exact: true }).click();
  await expect(page).toHaveURL(/\/blueprints/);
  await page.getByRole("link", { name: "Agents", exact: true }).click();
  await expect(page).toHaveURL(/\/agents/);

  const persistedScopeSwitcher = page.getByRole("button", { name: "Switch account" });
  await expect(persistedScopeSwitcher).toContainText("Test Org");
  await persistedScopeSwitcher.click();
  await page.getByRole("menuitem").filter({ hasText: "testuser" }).click();
  await expect(page.getByPlaceholder("Search agents...")).toHaveValue("");
});

test("active agent card links to deployment detail", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  await expect(page.getByText("Slack Full Bot")).toBeVisible({ timeout: 10_000 });

  const card = page.locator("[data-deployment-id]").filter({ hasText: "Slack Full Bot" }).first();
  const manageLink = card.locator('a[href*="/agents/"]').first();
  const href = await manageLink.getAttribute("href");
  expect(href).toContain("/agents/");
});

test("deploys navigate to dashboard", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto("/deploy/testuser/code-reviewer", { waitUntil: "domcontentloaded" });

  await expect(page.getByRole("button", { name: /deploy/i })).toBeVisible({ timeout: 20_000 });
  await page.getByLabel("Openai Api Key").fill("sk-test-value");
  await page.getByRole("button", { name: /slack/i }).click();
  await page.getByLabel("Slack App Token").fill("xapp-test-value");

  await Promise.all([
    page.waitForURL("**/agents**", { timeout: 30_000 }),
    page.getByRole("button", { name: /deploy/i }).click(),
  ]);

  await expect(page).toHaveURL(/\/agents/);
});
