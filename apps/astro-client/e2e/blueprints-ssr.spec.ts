import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("the first personal-account blueprint page is server-rendered without a hydration refetch", async ({
  page,
  request,
}) => {
  const response = await request.get("/blueprints");
  expect(response.status()).toBe(200);
  const html = await response.text();

  expect(html).toContain("code-reviewer");
  expect(html).not.toContain("org-support-bot");

  const browserListRequests: string[] = [];
  page.on("request", (browserRequest) => {
    if (new URL(browserRequest.url()).pathname === "/api/v1/me/blueprints") {
      browserListRequests.push(browserRequest.url());
    }
  });

  await page.goto("/blueprints", { waitUntil: "networkidle" });
  await expect(page.getByText("code-reviewer", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("org-support-bot", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Filter by account" })).toContainText("testuser");
  expect(browserListRequests).toEqual([]);
});

test("the account filter changes page-local scope without switching the active account", async ({
  context,
  page,
}) => {
  await page.goto("/blueprints", { waitUntil: "networkidle" });
  expect((await context.cookies()).some((cookie) => cookie.name === "astro:active-account")).toBe(false);

  await page.getByRole("button", { name: "Filter by account" }).click();
  const accountMenu = page.locator('[data-slot="multi-select-content"]');
  await accountMenu.getByRole("button", { name: /testuser/ }).click();
  await accountMenu.getByRole("button", { name: /Test Org/ }).click();

  await expect(page).toHaveURL(/\?account=test-org$/);
  await expect(page.getByText("org-support-bot", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("code-reviewer", { exact: true })).toHaveCount(0);
  expect((await context.cookies()).some((cookie) => cookie.name === "astro:active-account")).toBe(false);
});

test("agents and knowledge stores default to the personal account", async ({ page }) => {
  await page.goto("/agents", { waitUntil: "networkidle" });
  await expect(page.getByText("Slack Full Bot", { exact: true })).toBeVisible();
  await expect(page.getByText("Org Support Bot", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Filter by account" })).toContainText("testuser");

  await page.goto("/knowledge", { waitUntil: "networkidle" });
  await expect(page.getByText("shared-postgres", { exact: true })).toBeVisible();
  await expect(page.getByText("org-postgres", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Filter by account" })).toContainText("testuser");
});

test("the Knowledge Store account filter persists across route changes", async ({ page }) => {
  await page.goto("/knowledge", { waitUntil: "networkidle" });

  await page.getByRole("button", { name: "Filter by account" }).click();
  const accountMenu = page.locator('[data-slot="multi-select-content"]');
  await accountMenu.getByRole("button", { name: /testuser/ }).click();
  await accountMenu.getByRole("button", { name: /Test Org/ }).click();
  await expect(page).toHaveURL(/\/knowledge\?account=test-org$/);

  await page.getByRole("link", { name: "Blueprints", exact: true }).click();
  await expect(page).toHaveURL(/\/blueprints/);
  const knowledgeLink = page.getByRole("link", { name: "Knowledge", exact: true });
  await expect(knowledgeLink).toHaveAttribute("href", "/knowledge?account=test-org");
  await knowledgeLink.click();

  await expect(page).toHaveURL(/\/knowledge\?account=test-org$/);
  await expect(page.getByRole("button", { name: "Filter by account" })).toContainText("Test Org");
  await expect(page.getByText("org-postgres", { exact: true })).toBeVisible();
});

test("the Blueprint account filter persists across route changes", async ({ page }) => {
  await page.goto("/blueprints", { waitUntil: "networkidle" });

  await page.getByRole("button", { name: "Filter by account" }).click();
  const accountMenu = page.locator('[data-slot="multi-select-content"]');
  await accountMenu.getByRole("button", { name: /testuser/ }).click();
  await accountMenu.getByRole("button", { name: /Test Org/ }).click();
  await expect(page).toHaveURL(/\?account=test-org$/);

  await page.getByRole("link", { name: "Agents", exact: true }).click();
  await expect(page).toHaveURL(/\/agents$/);
  const blueprintsLink = page.getByRole("link", { name: "Blueprints", exact: true });
  await expect(blueprintsLink).toHaveAttribute("href", "/blueprints?account=test-org");
  await blueprintsLink.click();

  await expect(page).toHaveURL(/\/blueprints\?account=test-org$/);
  await expect(page.getByRole("button", { name: "Filter by account" })).toContainText("Test Org");
  await expect(page.getByText("org-support-bot", { exact: true }).first()).toBeVisible();
});
