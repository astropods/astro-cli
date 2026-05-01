import { test, expect } from "@playwright/test";
import { envConfig } from "./env";

test.describe("Homepage (/)", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/", { waitUntil: "load" });
  });

  test("loads with hero content", async ({ page }) => {
    await expect(page).toHaveTitle(/Astro AI/i);
    await expect(page.getByText("Agent-native infrastructure")).toBeVisible();
  });

  test("install command is visible", async ({ page }) => {
    await expect(page.getByRole("button", { name: "Copy install command" })).toBeVisible();
  });

  test("Get started button links to signup", async ({ page }) => {
    const btn = page.getByRole("link", { name: /get started/i }).first();
    await expect(btn).toBeVisible();
    await expect(btn).toHaveAttribute("href", /signup/);
  });

  test("Docs nav link is present", async ({ page }) => {
    await expect(page.getByRole("link", { name: "Docs" }).first()).toBeVisible();
  });

  test("Blog nav link is present", async ({ page }) => {
    await expect(page.getByRole("link", { name: "Blog" }).first()).toBeVisible();
  });
});

test.describe("Explore page (/explore)", () => {
  test("/auth/me returns 401 for unauthenticated users", async ({ page }) => {
    const authMeResponse = page.waitForResponse((res) => res.url().endsWith("/auth/me"), { timeout: 10000 });
    await page.goto("/explore", { waitUntil: "load" });
    const res = await authMeResponse;
    expect(res.status(), "/auth/me should return 401 for unauthenticated users").toBe(401);
  });

  test("no unexpected API errors", async ({ page }) => {
    const apiErrors: string[] = [];
    page.on("response", (res) => {
      const url = res.url();
      if (!url.endsWith("/auth/me") && url.includes(envConfig.apiDomain) && res.status() >= 400) {
        apiErrors.push(`${res.status()} ${url}`);
      }
    });
    await page.goto("/explore", { waitUntil: "load" });
    expect(apiErrors, `Unexpected API errors: ${apiErrors.join(", ")}`).toHaveLength(0);
  });

  test("shows at least 7 agent cards", async ({ page }) => {
    await page.goto("/explore", { waitUntil: "load" });
    const cards = page.locator("[class*='card'], article").filter({ hasText: /deploy/i });
    expect(await cards.count()).toBeGreaterThanOrEqual(envConfig.minExploreCards);
  });

  test("weather-poet card shows name, description, author and deploy count", async ({ page }) => {
    await page.goto("/explore", { waitUntil: "load" });

    const card = page.locator("a[href*='weather-poet']");
    await expect(card).toBeVisible();
    await expect(card.getByRole("heading", { name: "weather-poet" })).toBeVisible();
    await expect(card.getByText("Weather poet agent that delivers real-time forecasts for any city as a limerick.")).toBeVisible();
    await expect(card.getByText("rabbah")).toBeVisible();

    const deployText = await card.locator("*").filter({ hasText: /^\d+ deploys?$/ }).first().textContent();
    const deployCount = parseInt(deployText?.match(/(\d+)/)?.[1] ?? "0");
    expect(deployCount, `deploy count should be at least ${envConfig.minWeatherPoetDeploys}`).toBeGreaterThanOrEqual(envConfig.minWeatherPoetDeploys);
  });
});

test.describe("JSON specs", () => {
  test("astropods package schema is reachable and valid", async ({ request }) => {
    const res = await request.get("/schema/package.json");
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body["$id"]).toBe("https://astropods.com/schema/package.json");
    expect(body["$schema"]).toContain("json-schema.org");
    expect(body.properties).toBeDefined();
  });
});

test.describe("External links", () => {
  test("docs loads with content", async ({ page }) => {
    await page.goto("https://docs.astropods.com/welcome", { waitUntil: "load" });
    await expect(page).not.toHaveTitle(/404|not found/i);
    await expect(page.locator("h1, h2, article, main").first()).toBeVisible();
  });

  test("blog loads with content", async ({ page }) => {
    await page.goto("https://blog.astropods.com/", { waitUntil: "load" });
    await expect(page).not.toHaveTitle(/404|not found/i);
    await expect(page.locator("h1, h2, article, main").first()).toBeVisible();
  });
});
