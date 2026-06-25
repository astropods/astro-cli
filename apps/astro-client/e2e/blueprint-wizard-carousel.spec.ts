import { expect, test } from "@playwright/test";
import { expectEventually, resetMockBackend } from "./helpers";

test.beforeEach(async () => {
  await resetMockBackend();
});

// Regression: the step carousel viewport used overflow-hidden, which is still a
// scroll container — the browser auto-scrolled it to reveal a focused button in
// an off-screen slide, knocking the active slide out of alignment (blank/offset
// step). overflow-clip removes the scroll container so it can't happen.
test("wizard carousel stays aligned after navigating back then forward", async ({ page }) => {
  await page.goto("/new/custom", { waitUntil: "domcontentloaded" });

  await page.getByPlaceholder("my-agent").fill("mynewagent");
  await expectEventually(async () => {
    await expect(page.getByRole("button", { name: /^continue$/i })).toBeEnabled();
  });
  await page.getByRole("button", { name: /^continue$/i }).click();

  // On the source step: pick GitHub (focuses a button), then go back and forward.
  await page.getByText(/set up with github/i).click();
  await page.getByRole("button", { name: /^back$/i }).click();
  await page.getByRole("button", { name: /^continue$/i }).click();

  // The source step content must be aligned in the viewport, not scrolled away.
  await expect(page.getByText("Starting point")).toBeInViewport();

  // Direct check: the carousel viewport must not be horizontally scrolled.
  const scrollLeft = await page.evaluate(() => {
    const track = document.querySelector('[style*="translateX"]');
    return (track?.parentElement as HTMLElement | null)?.scrollLeft ?? -1;
  });
  expect(scrollLeft).toBe(0);
});
