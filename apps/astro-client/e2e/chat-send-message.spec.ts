import { test, expect } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// Chat main path: open a conversation, send a message, and watch the assistant
// reply stream back. The mock backend (e2e/mock-backend.ts) serves the send POST
// and the SSE response stream; ASSISTANT_REPLY below must match the reply it
// streams and persists.

const ID = "dep-slack-full-1";
const CONV = "conv-demo-1";
const CHAT_URL = `/chat/${ID}?conversation=${CONV}`;
const ASSISTANT_REPLY = /here is a quick plan for your trip\./;

test.describe("chat send message (main path)", () => {
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

  test("sends a message and streams the assistant reply", async ({ page }) => {
    await page.goto(CHAT_URL, { waitUntil: "domcontentloaded" });

    // Seeded history renders, so the thread is loaded and the composer is ready.
    await expect(page.getByText("Plan me a weekend in Lisbon.")).toBeVisible();
    const input = page.getByLabel("Message input");
    await expect(input).toBeEnabled();

    const prompt = "What should I do in Porto?";
    await input.fill(prompt);
    await page.getByRole("button", { name: "Send message" }).click();

    // The user turn shows immediately (optimistic) and the assistant reply
    // streams in over SSE.
    await expect(page.getByText(prompt)).toBeVisible();
    await expect(page.getByText(ASSISTANT_REPLY)).toBeVisible();

    // The turn finishes and the composer re-arms: Send returns, Stop is gone,
    // the input clears, and the user turn survives the post-finish history
    // refetch — proving the message round-tripped to the backend.
    await expect(page.getByRole("button", { name: "Send message" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Stop generating" })).toHaveCount(0);
    await expect(input).toHaveValue("");
    await expect(page.getByText(prompt)).toBeVisible();
  });

  test("supports a follow-up turn in the same conversation", async ({ page }) => {
    await page.goto(CHAT_URL, { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Plan me a weekend in Lisbon.")).toBeVisible();

    const input = page.getByLabel("Message input");
    const send = page.getByRole("button", { name: "Send message" });

    await input.fill("First question?");
    await send.click();
    await expect(page.getByText("First question?")).toBeVisible();
    await expect(page.getByText(ASSISTANT_REPLY)).toBeVisible();

    // Composer re-arms before the follow-up.
    await expect(send).toBeVisible();
    await input.fill("Second question?");
    await send.click();
    await expect(page.getByText("Second question?")).toBeVisible();

    // Both turns' replies are present — the thread accumulated across turns.
    await expect(page.getByText(ASSISTANT_REPLY)).toHaveCount(2);
  });
});
