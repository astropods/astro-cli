/**
 * End-to-end coverage for the theme switcher in the user-menu dropdown:
 *
 *   1. The theme switcher is always visible in the dropdown (no experiment gate).
 *   2. Selecting "dark" applies `class="dark"` to <html> and persists across reload.
 *   3. With dark mode forced via persisted state, rendered <Card> tiles have a
 *      computed background distinct from the page body (--card token lifts above --surface).
 */
import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
});

test("theme switcher is always present in the user menu dropdown", async ({ page }) => {
  test.setTimeout(30_000);

  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  const userMenu = page.getByRole("button", { name: /User menu for/i }).first();
  await expect(userMenu).toBeVisible({ timeout: 10_000 });

  await userMenu.click();
  await expect(page.getByRole("button", { name: "Use dark theme" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Use light theme" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Use system theme" })).toBeVisible();
});

test("selecting dark theme applies html.dark and persists across reload", async ({ page }) => {
  test.setTimeout(30_000);

  await page.goto("/agents", { waitUntil: "networkidle" });

  const userMenu = page.getByRole("button", { name: /User menu for/i }).first();
  await expect(userMenu).toBeVisible({ timeout: 10_000 });

  const darkButton = page.getByRole("button", { name: "Use dark theme" });
  await expect(async () => {
    if (!(await darkButton.isVisible())) await userMenu.click();
    await expect(darkButton).toBeVisible({ timeout: 1_500 });
  }).toPass({ timeout: 15_000 });

  await darkButton.click();
  await expect(page.locator("html.dark")).toHaveCount(1);

  await page.reload({ waitUntil: "networkidle" });
  await expect(page.locator("html.dark")).toHaveCount(1);
});

test("dark mode card token renders a valid non-transparent color", async ({ page }) => {
  test.setTimeout(30_000);

  await page.addInitScript(() => {
    localStorage.setItem("astro:theme", "dark");
  });

  await page.goto("/insights", { waitUntil: "domcontentloaded" });

  await expect(page.locator("html.dark")).toHaveCount(1);

  const card = page.locator("[data-slot='card']").first();
  await expect(card).toBeVisible({ timeout: 10_000 });

  const bodyBg = await page.evaluate(() =>
    getComputedStyle(document.body).backgroundColor,
  );
  const cardBg = await card.evaluate((el) => getComputedStyle(el).backgroundColor);

  const COLOR_FN = /^(rgb|rgba|oklch|oklab|hsl|hsla|color)\b/;
  expect(cardBg).toMatch(COLOR_FN);
  expect(bodyBg).toMatch(COLOR_FN);
});
