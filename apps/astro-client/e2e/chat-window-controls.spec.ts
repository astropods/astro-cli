import { test, expect } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// Chat window controls (#1355): auto-focus, open-in-new-tab, and the header title.

const ID = "dep-slack-full-1";
const CONV = "conv-demo-1";
const CHAT_URL = `/chat/${ID}?conversation=${CONV}`;

test.describe("chat window controls (#1355)", () => {
  test.beforeEach(async ({ page }) => {
    await resetMockBackend();
    // Force the messaging endpoint reachable so the composer is "ready".
    await page.route(new RegExp(`/deployments/${ID}/runtime$`), (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          runtime: { ready: 1, replicas: 1, messaging_reachable: true, workloads: [] },
        }),
      }),
    );
  });

  test("auto-focuses the composer on launch", async ({ page }) => {
    await page.goto(CHAT_URL, { waitUntil: "domcontentloaded" });

    // History renders, so the chat page fully loaded.
    await expect(page.getByText("Plan me a weekend in Lisbon.")).toBeVisible();

    const input = page.getByLabel("Message input");
    await expect(input).toBeVisible();
    await expect(input).toBeEnabled();
    await expect(input).toBeFocused();
  });

  test("shows the active conversation title and an open-in-new-tab control", async ({
    page,
  }) => {
    await page.goto(CHAT_URL, { waitUntil: "domcontentloaded" });

    const header = page.locator("header");
    await expect(header.getByText("Trip planning to Lisbon")).toBeVisible();

    const newTab = page.getByRole("link", { name: "Open chat in new tab" });
    await expect(newTab).toBeVisible();
    await expect(newTab).toHaveAttribute("href", CHAT_URL);
    await expect(newTab).toHaveAttribute("target", "_blank");
  });
});
