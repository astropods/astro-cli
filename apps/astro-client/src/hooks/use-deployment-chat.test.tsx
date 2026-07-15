import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { createHookWrapper } from "@/test/test-utils";
import { chatKeys } from "@/api/queries/keys";
import { useDeploymentChat } from "./use-deployment-chat";

class MockEventSource {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  static instances: MockEventSource[] = [];

  url: string;
  readyState = MockEventSource.CONNECTING;
  private listeners = new Map<string, Set<(event: Event) => void>>();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: Event) => void) {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set());
    }
    this.listeners.get(type)!.add(listener);
  }

  close() {
    this.readyState = MockEventSource.CLOSED;
    // A real closed EventSource delivers no further events; drop listeners so a
    // test emitting on a torn-down stream is correctly a no-op.
    this.listeners.clear();
  }

  static reset() {
    MockEventSource.instances = [];
  }

  static latest(): MockEventSource {
    const last = MockEventSource.instances.at(-1);
    if (!last) throw new Error("No MockEventSource instances");
    return last;
  }

  emit(type: string, data: string) {
    const listeners = this.listeners.get(type);
    if (!listeners) return;
    const event = new MessageEvent(type, { data });
    listeners.forEach((listener) => listener(event));
  }
}

const deploymentId = "dep-chat-1";
const newConversationId = "ef382a6b-c6c7-4a3e-a57b-b6832759f136";

function messagingHandlers() {
  server.use(
    http.post(
      `/api/v1/deployments/${deploymentId}/messaging/conversations`,
      () =>
        HttpResponse.json({ conversation_id: newConversationId }),
    ),
    http.post(
      `/api/v1/deployments/${deploymentId}/messaging/conversations/${newConversationId}/messages`,
      () => HttpResponse.json({ status: "ok" }),
    ),
  );
}

describe("useDeploymentChat", () => {
  beforeEach(() => {
    MockEventSource.reset();
    vi.stubGlobal("EventSource", MockEventSource);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("lazy create shows user message and streams assistant reply from SSE", async () => {
    messagingHandlers();

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: null }),
      { wrapper },
    );

    await act(async () => {
      await result.current.sendMessage("hello");
    });

    expect(result.current.messages.at(-1)?.content).toBe("hello");
    expect(MockEventSource.instances.length).toBeGreaterThanOrEqual(1);

    act(() => {
      MockEventSource.latest().emit(
        "chunk",
        JSON.stringify({ type: "chunk", content: "hi there" }),
      );
    });

    await waitFor(() => {
      expect(result.current.messages.at(-1)?.content).toBe("hi there");
    });

    act(() => {
      MockEventSource.latest().emit(
        "finish",
        JSON.stringify({ type: "finish" }),
      );
    });

    await waitFor(() => {
      expect(result.current.isStreaming).toBe(false);
    });
  });

  it("reflects the switched-to conversation's streaming state in place (no remount)", async () => {
    // Guards the fix in ChatWorkspace that keys the chat runtime on the agent, not
    // the conversation, so switching conversations re-scopes this hook in place
    // instead of remounting it. If the hook didn't react to a conversationId
    // change, the Stop/Send button would carry the previous chat's running state.
    const convStreaming = "11111111-1111-4111-8111-111111111111";
    const convIdle = "22222222-2222-4222-8222-222222222222";
    server.use(
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convStreaming}`,
        () =>
          HttpResponse.json({
            conversation_id: convStreaming,
            messages: [
              { id: "u1", role: "user", content: "hi" },
              { id: "a1", role: "assistant", content: "typing" },
            ],
            assistant_streaming: true,
          }),
      ),
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convIdle}`,
        () =>
          HttpResponse.json({
            conversation_id: convIdle,
            messages: [
              { id: "u2", role: "user", content: "yo" },
              { id: "a2", role: "assistant", content: "done" },
            ],
            assistant_streaming: false,
          }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result, rerender } = renderHook(
      ({ cid }: { cid: string }) =>
        useDeploymentChat(deploymentId, { conversationId: cid }),
      { wrapper, initialProps: { cid: convStreaming } },
    );

    // The streaming conversation drives the Stop button.
    await waitFor(() => expect(result.current.isStreaming).toBe(true));

    // Switching in place must flip to the idle conversation's state, not keep the
    // previous one's — i.e. the Stop button must not carry over.
    rerender({ cid: convIdle });
    await waitFor(() => expect(result.current.isStreaming).toBe(false));
  });

  it("keeps a conversation's stream alive across a switch so a finish while away clears Stop on return", async () => {
    // A turn that finishes while the user is viewing another conversation must
    // still be observed: its stream stays open across the switch and finalizes
    // the conversation in place, so returning shows the finished state rather
    // than a lingering Stop button.
    const convA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
    const convB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
    let aResponse = {
      conversation_id: convA,
      messages: [
        { id: "ua", role: "user", content: "hi" },
        { id: "aa", role: "assistant", content: "typing" },
      ],
      assistant_streaming: true,
    };
    server.use(
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convA}`,
        () => HttpResponse.json(aResponse),
      ),
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convB}`,
        () =>
          HttpResponse.json({
            conversation_id: convB,
            messages: [
              { id: "ub", role: "user", content: "yo" },
              { id: "ab", role: "assistant", content: "done" },
            ],
            assistant_streaming: false,
          }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result, rerender } = renderHook(
      ({ cid }: { cid: string }) =>
        useDeploymentChat(deploymentId, { conversationId: cid }),
      { wrapper, initialProps: { cid: convA } },
    );

    // A is streaming — Stop shown, stream open.
    await waitFor(() => expect(result.current.isStreaming).toBe(true));
    const aStream = MockEventSource.latest();

    // Switch away to an idle conversation.
    rerender({ cid: convB });
    await waitFor(() => expect(result.current.isStreaming).toBe(false));

    // A finishes while we're on B: the server marks it done and its stream (which
    // must still be alive) delivers the finish.
    aResponse = {
      conversation_id: convA,
      messages: [
        { id: "ua", role: "user", content: "hi" },
        { id: "aa", role: "assistant", content: "all done" },
      ],
      assistant_streaming: false,
    };
    act(() => {
      aStream.emit("finish", JSON.stringify({ type: "finish" }));
    });

    // Returning to A must not show a stale Stop button.
    rerender({ cid: convA });
    await waitFor(() => expect(result.current.isStreaming).toBe(false));
  });

  it("clears the history list's reply-in-progress dot when a turn finishes", async () => {
    const convId = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
    server.use(
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convId}`,
        () =>
          HttpResponse.json({
            conversation_id: convId,
            messages: [
              { id: "u1", role: "user", content: "hi" },
              { id: "a1", role: "assistant", content: "typing" },
            ],
            assistant_streaming: true,
          }),
      ),
    );

    const { wrapper, queryClient } = createHookWrapper();
    // The history list is a separate query; seed it showing this conversation
    // mid-reply (dot on).
    queryClient.setQueryData(chatKeys.conversations(deploymentId), {
      conversations: [
        {
          conversation_id: convId,
          title: "Chat",
          updated_at: "2026-07-10T00:00:00Z",
          assistant_streaming: true,
        },
      ],
    });

    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: convId }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.isStreaming).toBe(true));

    act(() => {
      MockEventSource.latest().emit(
        "finish",
        JSON.stringify({ type: "finish" }),
      );
    });

    await waitFor(() => {
      const list = queryClient.getQueryData(
        chatKeys.conversations(deploymentId),
      ) as { conversations: Array<{ assistant_streaming?: boolean }> };
      expect(list.conversations[0].assistant_streaming).toBe(false);
    });
  });

  it("sets streamError when send fails", async () => {
    messagingHandlers();
    server.use(
      http.post(
        `/api/v1/deployments/${deploymentId}/messaging/conversations/${newConversationId}/messages`,
        () =>
          HttpResponse.json({ error: "upstream failed" }, { status: 502 }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () =>
        useDeploymentChat(deploymentId, {
          conversationId: newConversationId,
        }),
      { wrapper },
    );

    await act(async () => {
      await result.current.sendMessage("hello");
    });

    expect(result.current.streamError).toBeTruthy();
    expect(result.current.isStreaming).toBe(false);
  });

  it("watchdog reaps a stalled turn so a resend opens a fresh stream", async () => {
    // A turn whose stream never delivers a finish (a stalled or reaped-but-not-
    // closed sidecar generation) must be reaped by the per-stream watchdog: the
    // EventSource is closed and removed so a resend isn't short-circuited by the
    // "already streaming" guard into reusing the dead stream. Regression guard for
    // the in-flight timeout that left the stream open and blocked retries.
    vi.useFakeTimers({ shouldAdvanceTime: true });
    messagingHandlers();

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: null }),
      { wrapper },
    );

    await act(async () => {
      await result.current.sendMessage("hello");
    });

    expect(MockEventSource.instances.length).toBe(1);
    const stalledStream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // No finish ever arrives; advance past the watchdog (IN_FLIGHT_TIMEOUT_MS,
    // 3 min).
    act(() => {
      vi.advanceTimersByTime(3 * 60 * 1000 + 1000);
    });

    expect(result.current.streamError).toBeTruthy();
    expect(result.current.isStreaming).toBe(false);
    // The watchdog must close the stalled stream, not merely reset UI state.
    expect(stalledStream.readyState).toBe(MockEventSource.CLOSED);

    // Resending must open a brand-new stream rather than reuse the dead one.
    await act(async () => {
      await result.current.sendMessage("again");
    });

    expect(MockEventSource.instances.length).toBe(2);
    expect(result.current.isStreaming).toBe(true);
  });
});
