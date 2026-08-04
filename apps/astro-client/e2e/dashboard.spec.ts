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

  const filteredResponse = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname === "/api/v1/me/deployments" && url.searchParams.get("q") === "full";
  });
  await page.getByPlaceholder("Search agents...").fill("full");
  await filteredResponse;

  await expect(page.getByText("Slack Full Bot")).toBeVisible({ timeout: 5_000 });
  await expect(page.getByText("Slack Overlap Bot")).not.toBeVisible();
});

test("dashboard account filter persists while search remains page-local", async ({ page }) => {
  test.setTimeout(30_000);
  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  const accountFilter = page.getByRole("button", { name: "Filter by account" });
  await expect(accountFilter).toBeVisible({ timeout: 10_000 });
  await accountFilter.click();
  const accountMenu = page.locator('[data-slot="multi-select-content"]');
  await accountMenu.getByRole("button", { name: /testuser/ }).click();
  await accountMenu.getByRole("button", { name: /Test Org/ }).click();

  await expect(page).toHaveURL(/\/agents\?account=test-org$/);
  await expect(accountFilter).toContainText("Test Org");
  await expect(page.getByText("Org Support Bot")).toBeVisible({ timeout: 10_000 });

  const search = page.getByPlaceholder("Search agents...");
  await search.fill("support");
  await expect(page.getByText("Org Support Bot")).toBeVisible();

  await page.getByRole("link", { name: "Blueprints", exact: true }).click();
  await expect(page).toHaveURL(/\/blueprints/);
  const agentsLink = page.getByRole("link", { name: "Agents", exact: true });
  await expect(agentsLink).toHaveAttribute("href", "/agents?account=test-org");
  await agentsLink.click();
  await expect(page).toHaveURL(/\/agents\?account=test-org$/);

  await expect(page.getByRole("button", { name: "Filter by account" })).toContainText("Test Org");
  await expect(page.getByPlaceholder("Search agents...")).toHaveValue("");
  await expect(page.getByText("Org Support Bot")).toBeVisible({ timeout: 10_000 });
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
