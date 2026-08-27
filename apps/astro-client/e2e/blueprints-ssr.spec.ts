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
  await expect(page.getByRole("combobox", { name: "Scope by account" })).toContainText("testuser");
  expect(browserListRequests).toEqual([]);
});

test("the org switcher re-scopes the session and the whole app", async ({ context, page }) => {
  const switchRequests: string[] = [];
  page.on("request", (request) => {
    if (new URL(request.url()).pathname === "/auth/switch-org") {
      switchRequests.push(request.postData() ?? "");
    }
  });

  await page.goto("/blueprints", { waitUntil: "networkidle" });
  expect((await context.cookies()).some((cookie) => cookie.name === "astro:active-account")).toBe(false);

  await page.getByRole("combobox", { name: "Scope by account" }).click();
  await page.getByRole("option", { name: /Test Org/ }).click();

  await expect(page.getByText("org-support-bot", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("code-reviewer", { exact: true })).toHaveCount(0);
  await expect(page).toHaveURL(/\/blueprints$/);
  expect(switchRequests.some((body) => body.includes("wos-org-1"))).toBe(true);
  expect(
    (await context.cookies()).find((cookie) => cookie.name === "astro:active-account")?.value,
  ).toBe("test-org");
});

test("agents and knowledge stores default to the personal account", async ({ page }) => {
  await page.goto("/agents", { waitUntil: "networkidle" });
  await expect(page.getByText("Slack Full Bot", { exact: true })).toBeVisible();
  await expect(page.getByText("Org Support Bot", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "Scope by account" })).toContainText("testuser");

  await page.goto("/knowledge", { waitUntil: "networkidle" });
  await expect(page.getByText("shared-postgres", { exact: true })).toBeVisible();
  await expect(page.getByText("org-postgres", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "Scope by account" })).toContainText("testuser");
});

test("the switched organization holds across primitives", async ({ page }) => {
  await page.goto("/knowledge", { waitUntil: "networkidle" });

  await page.getByRole("combobox", { name: "Scope by account" }).click();
  await page.getByRole("option", { name: /Test Org/ }).click();
  await expect(page.getByText("org-postgres", { exact: true })).toBeVisible();

  const blueprintsLink = page.getByRole("link", { name: "Blueprints", exact: true });
  await expect(blueprintsLink).toHaveAttribute("href", "/blueprints");
  await blueprintsLink.click();

  await expect(page).toHaveURL(/\/blueprints$/);
  await expect(page.getByRole("combobox", { name: "Scope by account" })).toContainText("Test Org");
  await expect(page.getByText("org-support-bot", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("code-reviewer", { exact: true })).toHaveCount(0);
});

test("an account deep link adopts that organization", async ({ page }) => {
  await page.goto("/blueprints?account=test-org", { waitUntil: "networkidle" });

  await expect(page.getByRole("combobox", { name: "Scope by account" })).toContainText("Test Org");
  await expect(page).toHaveURL(/\/blueprints$/);
  await expect(page.getByText("org-support-bot", { exact: true }).first()).toBeVisible();
});
