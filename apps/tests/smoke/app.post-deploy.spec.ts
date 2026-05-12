import { test, expect } from "./fixtures";

test(
  "hello-astro — deployment becomes active",
  { timeout: 900000 }, // 15 min: 14 min poll + headroom
  async ({ page }) => {
    await page.goto("/agents", { waitUntil: "load" });

    const helloAstroCard = page.locator("a").filter({ hasText: "Hello Astro" });
    await expect(helloAstroCard.getByText("Active", { exact: true })).toBeVisible({
      timeout: 840000,
      message: "Hello Astro deployment did not become Active within 14 minutes",
    } as any);
  }
);
