import { test, expect } from "@playwright/test";
import { resetMockBackend } from "./helpers";

// A fresh conversation's first (fast) reply must show a stable loading state —
// user bubble, then the "is replying" indicator, then the streamed reply — and
// never flicker (messages hydrated several times as a racing history refetch
// clobbers the in-flight turn). Standard locators can't catch flicker: they
// auto-retry and tolerate transient states. So we watch the DOM with a
// MutationObserver across the whole turn and assert nothing that appeared ever
// blinked back out.
//
// The race is reproduced deterministically: the just-created conversation's
// history GET returns an empty (lagging) thread while the stream is live, and
// the real thread only once the turn is persisted. On the unfixed client the
// mid-stream refetch replaces the cache with that empty thread and the user
// bubble / streamed reply vanish; the fix serves the cache while the stream is
// live, so the clobber never happens.

const ID = "dep-slack-full-1";
const CONV = "conv-fresh-1";
const PROMPT = "Plan a day trip to Sintra.";
const REPLY = "Sure, here is a quick plan for your trip.";

type FlickerState = {
  sawUser: boolean;
  userDisappeared: boolean;
  sawAssistant: boolean;
  assistantDisappeared: boolean;
  sawIndicator: boolean;
};

test.describe("chat loading stability (no flicker)", () => {
  test.beforeEach(async ({ page }) => {
    await resetMockBackend();

    // A turn goes: before -> streaming (stream live, history lags) -> done
    // (turn persisted, history returns the real thread).
    let phase: "before" | "streaming" | "done" = "before";

    // Composer ready.
    await page.route(new RegExp(`/deployments/${ID}/runtime$`), (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          runtime: { ready: 1, replicas: 1, messaging_reachable: true, workloads: [] },
        }),
      }),
    );

    // No existing conversations -> the workspace selects none -> the first send
    // lazily creates one (the create branch, where the query activates mid-send).
    await page.route(/\/chat\/conversations$/, (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ conversations: [] }) }),
    );

    // Lazy create returns a known id so we can key the routes below.
    await page.route(/\/messaging\/conversations$/, (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ conversation_id: CONV }) }),
    );
    await page.route(new RegExp(`/messaging/conversations/${CONV}/messages$`), (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ message_id: "m1" }) }),
    );

    // History: empty (lagging) until the turn is persisted. A mid-stream refetch
    // that honored this would wipe the optimistic + streamed rows. Match the
    // trailing ?limit= query the client sends (full fetch and tail merge both).
    await page.route(new RegExp(`/chat/conversations/${CONV}(\\?|$)`), (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          phase === "done"
            ? {
                conversation_id: CONV,
                title: "New chat",
                messages: [
                  { id: "u1", role: "user", content: PROMPT },
                  { id: "a1", role: "assistant", content: REPLY },
                ],
                has_more: false,
              }
            : { conversation_id: CONV, title: "New chat", messages: [], has_more: false },
        ),
      }),
    );

    // Keep the stream live for a window (so a racing history refetch is possible),
    // then deliver the reply + finish. Flip to "done" before delivering finish so
    // the finish-driven reconcile refetch returns the persisted thread.
    await page.route(new RegExp(`/messaging/conversations/${CONV}/stream$`), async (route) => {
      phase = "streaming";
      await new Promise((r) => setTimeout(r, 600));
      phase = "done";
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        headers: { "cache-control": "no-cache" },
        body:
          `event: chunk\ndata: ${JSON.stringify({ type: "chunk", content: REPLY })}\n\n` +
          `event: finish\ndata: {}\n\n`,
      });
    });
  });

  test("shows a stable loading state and never flickers on a fresh conversation", async ({
    page,
  }) => {
    await page.goto(`/chat/${ID}`, { waitUntil: "domcontentloaded" });

    const input = page.getByLabel("Message input");
    await expect(input).toBeEnabled();

    // Watch the whole turn for churn. Standard expects auto-retry and would miss
    // a bubble that blinks out and back; the observer records every transition.
    await page.evaluate(() => {
      const state = {
        sawUser: false,
        userDisappeared: false,
        sawAssistant: false,
        assistantDisappeared: false,
        sawIndicator: false,
      };
      (window as unknown as { __chatFlicker: typeof state }).__chatFlicker = state;
      const sample = () => {
        if (document.querySelector('[data-role="user"]')) state.sawUser = true;
        else if (state.sawUser) state.userDisappeared = true;

        const assistants = document.querySelectorAll('[data-role="assistant"]');
        const text = (assistants[assistants.length - 1]?.textContent ?? "").trim();
        if (text.length > 0) state.sawAssistant = true;
        else if (state.sawAssistant) state.assistantDisappeared = true;

        if (document.querySelector('[role="status"][aria-label*="is replying"]')) {
          state.sawIndicator = true;
        }
      };
      const observer = new MutationObserver(sample);
      observer.observe(document.body, { childList: true, subtree: true, characterData: true });
    });

    await input.fill(PROMPT);
    await page.getByRole("button", { name: "Send message" }).click();

    // The reply streams in and the turn completes (composer re-arms).
    await expect(page.getByText(REPLY)).toBeVisible();
    await expect(page.getByRole("button", { name: "Send message" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Stop generating" })).toHaveCount(0);
    // The user message is still there at the end (survived the reconcile refetch).
    await expect(page.getByText(PROMPT)).toBeVisible();

    const flicker = await page.evaluate(
      () => (window as unknown as { __chatFlicker: FlickerState }).__chatFlicker,
    );

    // The nice loading state and both messages were shown...
    expect(flicker.sawUser).toBe(true);
    expect(flicker.sawIndicator).toBe(true);
    expect(flicker.sawAssistant).toBe(true);
    // ...and neither blinked out mid-turn (no clobbering re-render).
    expect(flicker.userDisappeared).toBe(false);
    expect(flicker.assistantDisappeared).toBe(false);
  });
});
