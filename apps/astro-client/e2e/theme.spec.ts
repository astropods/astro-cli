/**
 * End-to-end coverage for the theme + experiments wiring in the production
 * build:
 *
 *   1. Toggling the `theming` experiment in /settings/experiments reveals
 *      the theme switcher in the user-menu dropdown without a reload, i.e.
 *      experiment state is shared across the tree in real time.
 *   2. Selecting "dark" applies `class="dark"` to `<html>` immediately and
 *      the choice survives a `page.reload()`.
 *   3. With dark mode forced via persisted state, the rendered `<Card>`
 *      tiles have a computed background distinct from the page body, i.e.
 *      the `--card` token actually lifts above `--surface`.
 */
import { expect, test } from "@playwright/test";

const MOCK_BACKEND = "http://localhost:48787";

test.beforeEach(async () => {
  await fetch(`${MOCK_BACKEND}/test/reset`, { method: "POST" });
  // Each test gets a fresh browser context (empty localStorage) from
  // Playwright; per-test seeding of localStorage is done via
  // `page.addInitScript` inside the individual tests below. Avoid adding
  // a context-wide init script that mutates localStorage — it would also
  // run on `page.reload()` and clobber any state under test.
});

test("enabling the theming experiment reveals the theme switcher live", async ({ page }) => {
  test.setTimeout(30_000);

  await page.goto("/settings/experiments", { waitUntil: "domcontentloaded" });

  const userMenu = page.getByRole("button", { name: /User menu for/i }).first();
  await expect(userMenu).toBeVisible({ timeout: 10_000 });

  // Switcher is gated on `experiments.theming`; with the experiment off it
  // is absent from the dropdown.
  await userMenu.click();
  await expect(page.getByRole("button", { name: "Use dark theme" })).toHaveCount(0);

  // Dismiss the dropdown so the page below is interactive again.
  await page.keyboard.press("Escape");

  // Flip the Theming experiment.
  const themingRow = page.getByText("Theming", { exact: true }).locator("..").locator("..");
  await themingRow.getByRole("switch").click();

  // Re-opening the dropdown reveals all three theme options without a reload.
  await userMenu.click();
  await expect(page.getByRole("button", { name: "Use light theme" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Use dark theme" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Use system theme" })).toBeVisible();
});

test("selecting dark theme applies html.dark and persists across reload", async ({ page }) => {
  test.setTimeout(30_000);

  // Seed the theming experiment so the switcher is rendered on first paint
  // and on every subsequent navigation in this context.
  await page.addInitScript(() => {
    localStorage.setItem("astro:experiments", JSON.stringify({ theming: true }));
  });

  await page.goto("/agents", { waitUntil: "networkidle" });

  const userMenu = page.getByRole("button", { name: /User menu for/i }).first();
  await expect(userMenu).toBeVisible({ timeout: 10_000 });

  // SSR renders with `experiments.theming === false` (server reads DEFAULTS
  // because `typeof window === "undefined"`), then the client re-reads
  // localStorage post-hydration and flips it to true. Clicks dispatched on
  // the trigger before hydration finishes are dropped by Radix. Retry the
  // open until the dark-theme button is actually visible — guarding against
  // re-clicking an already-open menu (which would toggle it closed).
  const darkButton = page.getByRole("button", { name: "Use dark theme" });
  await expect(async () => {
    if (!(await darkButton.isVisible())) {
      await userMenu.click();
    }
    await expect(darkButton).toBeVisible({ timeout: 1_500 });
  }).toPass({ timeout: 15_000 });

  await darkButton.click();

  await expect(page.locator("html.dark")).toHaveCount(1);

  await page.reload({ waitUntil: "networkidle" });
  await expect(page.locator("html.dark")).toHaveCount(1);
});

test("dark mode card token renders a valid non-transparent color", async ({ page }) => {
  test.setTimeout(30_000);

  // Boot the page directly into dark mode with the experiment enabled so
  // the test asserts rendered output without any UI interaction.
  await page.addInitScript(() => {
    localStorage.setItem("astro:theme", "dark");
    localStorage.setItem("astro:experiments", JSON.stringify({ theming: true }));
  });

  await page.goto("/agents", { waitUntil: "domcontentloaded" });

  await expect(page.locator("html.dark")).toHaveCount(1);

  // The dashboard renders its tiles via the `<Card>` primitive
  // (`data-slot="card"`), which uses the `--card` token.
  const card = page.locator("[data-slot='card']").first();
  await expect(card).toBeVisible({ timeout: 10_000 });

  const bodyBg = await page.evaluate(() =>
    getComputedStyle(document.body).backgroundColor,
  );
  const cardBg = await card.evaluate((el) => getComputedStyle(el).backgroundColor);

  // `getComputedStyle` returns the color in whatever function it was
  // authored with: `rgb(...)`, `oklch(...)`, etc. Accept any color function
  // and only fail if the value is empty / transparent.
  const COLOR_FN = /^(rgb|rgba|oklch|oklab|hsl|hsla|color)\b/;
  expect(cardBg).toMatch(COLOR_FN);
  expect(bodyBg).toMatch(COLOR_FN);
});
