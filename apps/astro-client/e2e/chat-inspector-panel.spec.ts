import { test, expect } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// The chat inspector is a docked SidePanel: the "Agent details" toggle slides it
// in, and "Hide panel" closes it. Regression for the shared shell owning the
// open/close animation and the small-screen sheet (#1635).
const ID = "dep-slack-full-1";
const CONV = "conv-demo-1";
const CHAT_URL = `/chat/${ID}?conversation=${CONV}`;

test.beforeEach(async () => {
  await resetMockBackend();
});

test("opens and closes the chat inspector (docked side panel)", async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(CHAT_URL, { waitUntil: "domcontentloaded" });

  await page.getByRole("button", { name: "Agent details" }).click();
  // Docked content is mounted: its footer link and its own close control show.
  await expect(page.getByRole("link", { name: /view agent/i })).toBeVisible();
  const hide = page.getByRole("button", { name: "Hide panel" });
  await expect(hide).toBeVisible();

  await hide.click();
  await expect(page.getByRole("link", { name: /view agent/i })).toBeHidden();
});
