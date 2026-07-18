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

// Verifies the returning-user marker is set by a real login. The login sets the
// durable `astro_returning` cookie server-side (astro-server: handlers/auth.go
// setReturningCookie) on the shared *.astropods.com origin, non-HttpOnly so the
// marketing nav can read it and swap "Get started" → "Sign in" for returning
// visitors.
//
// We only assert the cookie is set. The nav swap itself is NOT verified against
// prod: astropods.com is behind Cloudflare bot management, which serves automated
// browsers a response where client-side document.cookie is inert — so the nav's
// cookie read never fires under Playwright (headed or headless), even though real
// users get the correct CTA. The swap is covered by the website's own suite
// (modules/website/tests/rendering.spec.ts), which runs against the site directly.
test("login sets the returning-user cookie", async ({ page }) => {
  const cookies = await page.context().cookies();
  expect(
    cookies.some((c) => c.name === "astro_returning"),
    "login should set the astro_returning cookie",
  ).toBe(true);
});
