import { test, expect } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// Server-authoritative termination: when the sidecar emits an `error` SSE event
// (agent crashed / disconnected / stalled), the client ends the turn right away,
// shows the server's message, and re-arms the composer — no waiting on the
// client liveness watchdog.

const ID = "dep-slack-full-1";
const CONV = "conv-demo-1";
const CHAT_URL = `/chat/${ID}?conversation=${CONV}`;
const ERROR_MESSAGE = "The agent disconnected. You can try sending again.";

const sse = (event: string, data: unknown) =>
  `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;

test.describe("chat server error ends the turn", () => {
  test.beforeEach(async ({ page }) => {
    await resetMockBackend();
    // Composer is "ready" only when messaging is reachable.
    await page.route(new RegExp(`/deployments/${ID}/runtime$`), (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          runtime: { ready: 1, replicas: 1, messaging_reachable: true, workloads: [] },
        }),
      }),
    );
    // The SSE stream sends a chunk, then a terminal server `error` event.
    await page.route(/messaging\/conversations\/[^/]+\/stream/, (route) =>
      route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        headers: { "cache-control": "no-cache" },
        body:
          sse("chunk", { type: "chunk", content: "Working on it..." }) +
          sse("error", { type: "error", message: ERROR_MESSAGE, retryable: true }),
      }),
    );
  });

  test("surfaces the server error and re-arms the composer", async ({ page }) => {
    await page.goto(CHAT_URL, { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Plan me a weekend in Lisbon.")).toBeVisible();

    const input = page.getByLabel("Message input");
    await expect(input).toBeEnabled();
    await input.fill("Do something that fails");
    await page.getByRole("button", { name: "Send message" }).click();

    // The server's error message surfaces above the composer...
    await expect(page.getByText(ERROR_MESSAGE)).toBeVisible();
    // ...and the turn ends: composer re-arms and Stop is gone.
    await expect(page.getByRole("button", { name: "Send message" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Stop generating" })).toHaveCount(0);
  });
});
