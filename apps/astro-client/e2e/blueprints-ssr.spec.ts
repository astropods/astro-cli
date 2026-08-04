import { expect, test } from "@playwright/test";
import { resetMockBackend } from "./helpers";

test.beforeEach(async () => {
  await resetMockBackend();
});

test("the first all-account blueprint page is server-rendered without a hydration refetch", async ({
  page,
  request,
}) => {
  const response = await request.get("/blueprints");
  expect(response.status()).toBe(200);
  const html = await response.text();

  expect(html).toContain("code-reviewer");
  expect(html).toContain("org-support-bot");

  const browserListRequests: string[] = [];
  page.on("request", (browserRequest) => {
    if (new URL(browserRequest.url()).pathname === "/api/v1/me/blueprints") {
      browserListRequests.push(browserRequest.url());
    }
  });

  await page.goto("/blueprints", { waitUntil: "networkidle" });
  await expect(page.getByText("code-reviewer", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("org-support-bot", { exact: true }).first()).toBeVisible();
  expect(browserListRequests).toEqual([]);
});

test("the account filter changes page-local scope without switching the active account", async ({
  context,
  page,
}) => {
  await page.goto("/blueprints", { waitUntil: "networkidle" });
  expect((await context.cookies()).some((cookie) => cookie.name === "astro:active-account")).toBe(false);

  await page.getByRole("button", { name: "Filter by account" }).click();
  await page
    .locator('[data-slot="multi-select-content"]')
    .getByRole("button", { name: /Test Org/ })
    .click();

  await expect(page).toHaveURL(/\?account=test-org$/);
  await expect(page.getByText("org-support-bot", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("code-reviewer", { exact: true })).toHaveCount(0);
  expect((await context.cookies()).some((cookie) => cookie.name === "astro:active-account")).toBe(false);
});

test("agents and knowledge stores render resources from every account", async ({ page }) => {
  await page.goto("/agents", { waitUntil: "networkidle" });
  await expect(page.getByText("Slack Full Bot", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Filter by account" })).toContainText("All accounts");

  await page.goto("/knowledge", { waitUntil: "networkidle" });
  await expect(page.getByText("shared-postgres", { exact: true })).toBeVisible();
  await expect(page.getByText("org-postgres", { exact: true })).toBeVisible();
  await expect(page.getByText("Test Org", { exact: true })).toBeVisible();
});
