import { test, expect } from "./fixtures";
import { envConfig } from "./env";

// Smoke test: verifies the saved auth session actually grants access to the app.
test("authenticated product loads at root", async ({ page }) => {
  await page.goto("/", { waitUntil: "load" });

  // When authenticated, / routes to the app — not the marketing site or login page
  await expect(page).not.toHaveURL(envConfig.loginUrlPattern);

  // App chrome is visible
  await expect(page.getByRole("button", { name: /user menu for/i })).toBeVisible();
});
