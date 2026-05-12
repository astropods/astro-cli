import { test, expect } from "./fixtures";
import { envConfig } from "./env";

test("hello-astro — chat responds to a message", async ({ page }) => {
  await page.goto("/agents", { waitUntil: "load" });

  await page.getByText("Hello Astro").first().click();
  await page.waitForURL(new RegExp(`/${envConfig.username}/agents/[a-z0-9-]+`), { timeout: 15000 });

  // Chat opens via window.open() — use popup event on the originating page
  const chatPagePromise = page.waitForEvent("popup", { timeout: 30000 });
  await page.getByRole("button", { name: /^chat$/i }).click();
  const chatPage = await chatPagePromise;

  // Wait for the chat input directly rather than relying on networkidle
  await chatPage.getByPlaceholder(/send a message/i).waitFor({ state: "visible", timeout: 30000 });

  await chatPage.getByPlaceholder(/send a message/i).fill("hello astro");
  await chatPage.getByPlaceholder(/send a message/i).press("Enter");
  await expect(chatPage.getByText("Echo: hello astro")).toBeVisible({ timeout: 30000 });
});
