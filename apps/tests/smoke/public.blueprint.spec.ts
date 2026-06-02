import { test, expect } from "./fixtures";
import { envConfig } from "./env";

test.describe("weather-poet detail page", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/explore", { waitUntil: "load" });
    await page.locator("a[href*='weather-poet']").click();
    await page.waitForURL(/weather-poet/, { timeout: 15000 });
    await page.waitForLoadState("load");
  });

  test("shows agent name and tags", async ({ page }) => {
    await expect(page.getByRole("heading", { name: "weather-poet" })).toBeVisible();
    // exact: true ensures "weather" won't match "weather-poet" or "weatherapi.com"
    await expect(page.getByText("weather", { exact: true })).toBeVisible();
    await expect(page.getByText("poetry", { exact: true })).toBeVisible();
    await expect(page.getByText("mastra", { exact: true })).toBeVisible();
    await expect(page.getByText("example", { exact: true })).toBeVisible();
  });

  test("deploy button is present", async ({ page }) => {
    await expect(page.getByRole("link", { name: /deploy this agent/i })).toBeVisible();
  });


  test("shows repository", async ({ page }) => {
    await expect(page.getByRole("link", { name: "rabbah/weather-poet" })).toBeVisible();
  });

  test("shows integrations", async ({ page }) => {
    // .last() targets the desktop sidebar — the first copy is inside min-[900px]:hidden (mobile)
    await expect(page.getByText("Anthropic", { exact: true }).last()).toBeVisible();
    await expect(page.getByText("weatherapi.com", { exact: true }).last()).toBeVisible();
  });

  test("heart and share actions are present", async ({ page }) => {
    await expect(page.getByRole("button", { name: /heart|like|favorite/i }).or(
      page.locator("[aria-label*='heart'], [aria-label*='like'], [aria-label*='favorite']")
    )).toBeVisible();
    await expect(page.getByRole("button", { name: /share/i }).or(
      page.locator("[aria-label*='share']")
    )).toBeVisible();
  });

  test("shows contributor", async ({ page }) => {
    // .last() targets the desktop sidebar — the first copy is inside min-[900px]:hidden (mobile)
    await expect(page.getByText(envConfig.authorDisplayName).last()).toBeVisible();
    await expect(page.getByText("@rabbah", { exact: true }).last()).toBeVisible();
  });
});

test.describe("weather-poet deploy flow", () => {
  test("deploy button redirects to login", async ({ page }) => {
    const apiErrors: string[] = [];
    const authMeStatuses: number[] = [];
    page.on("response", (res) => {
      const url = res.url();
      if (url.endsWith("/auth/me")) {
        authMeStatuses.push(res.status());
      } else if (url.includes(envConfig.apiDomain) && res.status() >= 400) {
        apiErrors.push(`${res.status()} ${url}`);
      }
    });

    await page.goto("/rabbah/weather-poet", { waitUntil: "load" });
    await page.getByRole("link", { name: /deploy this agent/i }).last().click();
    await page.waitForURL(envConfig.loginUrlPattern, { timeout: 15000 });
    await expect(page).toHaveURL(envConfig.loginUrlPattern);
    expect(authMeStatuses[0], "/auth/me should return 401 for unauthenticated users").toBe(401);
    expect(apiErrors, `Unexpected API errors on deploy redirect: ${apiErrors.join(", ")}`).toHaveLength(0);
  });

});
