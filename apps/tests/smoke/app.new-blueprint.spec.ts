import { test, expect } from "./fixtures";

// Regression guard for the "Setup your agent blueprint" screen, which previously
// clamped its height with overflow-hidden and clipped the action buttons below
// the fold with no way to scroll to them. We use a short viewport so the bottom
// action is guaranteed off-screen, then a real wheel scroll (which an
// overflow-hidden container would NOT respond to) and assert the button is now
// in the viewport — i.e. the screen is genuinely scrollable.
test.describe("new blueprint — setup screen scroll", () => {
  test("action buttons are reachable by scrolling", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 600 });
    await page.goto("/new/custom", { waitUntil: "load" });

    // Page rendered (and we're authenticated, not bounced to login).
    await expect(
      page.getByRole("heading", { name: /set ?up your agent blueprint/i }),
    ).toBeVisible();

    // The primary action lives at the bottom of the form.
    const continueBtn = page.getByRole("button", { name: /^continue$/i });
    await expect(continueBtn).toBeAttached();

    // Simulate a user scrolling down. A wheel event only moves a scrollable
    // container, so this reveals the button only when the page actually scrolls.
    await page.mouse.move(640, 300);
    await page.mouse.wheel(0, 4000);

    await expect(continueBtn).toBeInViewport();
  });
});
