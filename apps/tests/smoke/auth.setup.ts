// The test account must be on the WorkOS captcha bypass allow list, otherwise login
// will be blocked by a CAPTCHA challenge. To add an account:
// WorkOS Dashboard → Radar → Configuration → Users (under Custom lists)
import path from "path";
import { test as setup } from "@playwright/test";
import { mkdir } from "fs/promises";
import { envConfig } from "./env";

export const authFile = path.join(import.meta.dirname, "playwright/.auth/user.json");

setup("authenticate", async ({ page }) => {
  const email = process.env.ASTRO_TEST_EMAIL;
  const password = process.env.ASTRO_TEST_PASSWORD;

  if (!email || !password) {
    throw new Error("ASTRO_TEST_EMAIL / ASTRO_TEST_PASSWORD not set — all app-site tests will be skipped");
  }

  await mkdir(path.join(import.meta.dirname, "playwright/.auth"), { recursive: true });

  // Navigate to a protected route so the app redirects to login (not signup)
  await page.goto("/settings", { waitUntil: "load", timeout: 60000 });
  await page.waitForURL(envConfig.loginUrlPattern, { timeout: 15000 });

  await page.getByLabel(/email/i).fill(email);
  await page.getByRole("button", { name: /continue/i }).click();

  await page.getByLabel(/password/i).waitFor({ state: "visible", timeout: 10000 });
  await page.getByLabel(/password/i).fill(password);
  // Anchored to avoid matching "Sign in with a passkey" on the preview login page
  await page.getByRole("button", { name: /^(continue|sign in)$/i }).click();

  // Race success vs invalid-credentials. The error branch must not reject on timeout —
  // Promise.race fails on the first rejection, so a slow OAuth redirect (>5s) would
  // flake even when login succeeds.
  const loginSucceeded = page.waitForURL(
    (url) => envConfig.loginUrlExclude(url.toString()),
    { timeout: 60000 },
  );
  const loginFailed = (async () => {
    try {
      await page.getByText(/invalid email or password/i).waitFor({ state: "visible", timeout: 5000 });
      throw new Error("Login failed: invalid credentials — check ASTRO_TEST_EMAIL / ASTRO_TEST_PASSWORD secrets");
    } catch (err) {
      if (err instanceof Error && err.message.startsWith("Login failed")) {
        throw err;
      }
      // No error text yet — keep this branch pending so loginSucceeded can win the race.
      await new Promise(() => {});
    }
  })();

  await Promise.race([loginSucceeded, loginFailed]);

  await page.context().storageState({ path: authFile });
});
