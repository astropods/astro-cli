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

  // A native connection error, as EventSource dispatches on each failed
  // reconnect: a plain Event on the "error" listener, not a MessageEvent.
  emitNativeError() {
    const listeners = this.listeners.get("error");
    if (!listeners) return;
    const event = new Event("error");
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

  it("does not clobber a fresh conversation's streamed reply with a racing history refetch", async () => {
    // A brand-new conversation activates the history query mid-send, and its GET
    // lags the live stream (nothing persisted yet). refetchOnMount:"always" must
    // be suppressed while the stream feeds the cache, or its full replace wipes
    // the optimistic user row and the streamed assistant chunks — the fresh-chat
    // flicker. Regression guard.
    let historyGets = 0;
    server.use(
      http.post(
        `/api/v1/deployments/${deploymentId}/messaging/conversations`,
        () => HttpResponse.json({ conversation_id: newConversationId }),
      ),
      http.post(
        `/api/v1/deployments/${deploymentId}/messaging/conversations/${newConversationId}/messages`,
        () => HttpResponse.json({ status: "ok" }),
      ),
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${newConversationId}`,
        () => {
          historyGets += 1;
          return HttpResponse.json({
            conversation_id: newConversationId,
            messages: [],
            assistant_streaming: false,
          });
        },
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: null }),
      { wrapper },
    );

    await act(async () => {
      await result.current.sendMessage("hello");
    });
    expect(result.current.messages.at(-1)?.content).toBe("hello");

    act(() => {
      MockEventSource.latest().emit(
        "chunk",
        JSON.stringify({ type: "chunk", content: "streamed reply" }),
      );
    });
    await waitFor(() =>
      expect(
        result.current.messages.some((m) => m.content === "streamed reply"),
      ).toBe(true),
    );

    // Let any racing refetch settle; with the fix it never fires.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });

    // Both the optimistic user row and the streamed reply survive, and the
    // clobbering mount fetch was served from cache instead of the network.
    expect(result.current.messages.some((m) => m.content === "hello")).toBe(true);
    expect(
      result.current.messages.some((m) => m.content === "streamed reply"),
    ).toBe(true);
    expect(historyGets).toBe(0);
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

  it("clears a reload-sourced interaction immediately on response, before the turn finishes", async () => {
    // Regression guard: on reload the form comes from the persisted queue, and the
    // cache-served query can't refetch it away mid-turn — resolving must clear it.
    const convId = "dddddddd-dddd-4ddd-8ddd-dddddddddddd";
    const interaction = {
      id: "int-reload-1",
      kind: "form",
      message: "Approve?",
      dataSchema: { type: "object", properties: {} },
      actions: ["submit"],
    };
    server.use(
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convId}`,
        () =>
          HttpResponse.json({
            conversation_id: convId,
            messages: [{ id: "u1", role: "user", content: "hi" }],
            assistant_streaming: true,
            pending_interactions: [interaction],
          }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: convId }),
      { wrapper },
    );

    await waitFor(() =>
      expect(result.current.pendingInteraction?.id).toBe("int-reload-1"),
    );

    act(() => {
      result.current.clearPendingInteraction();
    });
    expect(result.current.pendingInteraction).toBeNull();
  });

  it("clears each entry of a multi-interaction reload queue without resurfacing an earlier one", async () => {
    // FIFO queue with the cache frozen mid-turn: answering the second must not
    // resurface the first. Guards the resolved-id set vs the single-scalar bug.
    const convId = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee";
    const mk = (id: string) => ({
      id,
      kind: "form",
      message: `Approve ${id}?`,
      dataSchema: { type: "object", properties: {} },
      actions: ["submit"],
    });
    server.use(
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convId}`,
        () =>
          HttpResponse.json({
            conversation_id: convId,
            messages: [{ id: "u1", role: "user", content: "hi" }],
            assistant_streaming: true,
            pending_interactions: [mk("int-a"), mk("int-b")],
          }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: convId }),
      { wrapper },
    );

    await waitFor(() =>
      expect(result.current.pendingInteraction?.id).toBe("int-a"),
    );

    // Answer A → B advances into view.
    act(() => result.current.clearPendingInteraction());
    expect(result.current.pendingInteraction?.id).toBe("int-b");

    // Answer B → queue is empty; A must not resurface from the frozen cache.
    act(() => result.current.clearPendingInteraction());
    expect(result.current.pendingInteraction).toBeNull();
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

  it("liveness watchdog reaps a dead SSE pipe so a resend opens a fresh stream", async () => {
    // With no inbound activity at all (not even heartbeats) the pipe is dead; the
    // watchdog closes the EventSource so a resend isn't short-circuited by the
    // "already streaming" guard into reusing it.
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
    const deadStream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // No chunk and no heartbeat arrives; advance past the liveness window (90s).
    act(() => {
      vi.advanceTimersByTime(90 * 1000 + 1000);
    });

    expect(result.current.streamError).toBeTruthy();
    expect(result.current.isStreaming).toBe(false);
    expect(deadStream.readyState).toBe(MockEventSource.CLOSED);

    // Resending must open a brand-new stream rather than reuse the dead one.
    await act(async () => {
      await result.current.sendMessage("again");
    });

    expect(MockEventSource.instances.length).toBe(2);
    expect(result.current.isStreaming).toBe(true);
  });

  it("heartbeats keep a long, content-quiet turn alive", async () => {
    // A slow turn emits no content for minutes but the sidecar heartbeats every
    // 30s. Each heartbeat resets the liveness watchdog, so the turn keeps
    // streaming with no error. Regression guard for the old fixed whole-turn
    // timeout that cut off long turns even while the pipe was healthy.
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

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // Heartbeat every 30s for 6 min (well past the old 3-min cap), no content.
    for (let i = 0; i < 12; i++) {
      act(() => {
        vi.advanceTimersByTime(30 * 1000);
      });
      act(() => {
        stream.emit("heartbeat", JSON.stringify({ type: "heartbeat" }));
      });
      expect(result.current.streamError).toBeNull();
      expect(result.current.isStreaming).toBe(true);
      expect(stream.readyState).not.toBe(MockEventSource.CLOSED);
    }

    // Heartbeats stop; after a full silent window the watchdog reaps the pipe.
    act(() => {
      vi.advanceTimersByTime(90 * 1000 + 1000);
    });

    expect(result.current.streamError).toBeTruthy();
    expect(result.current.isStreaming).toBe(false);
    expect(stream.readyState).toBe(MockEventSource.CLOSED);
  });

  it("interaction events keep a content-quiet turn alive", async () => {
    // An agent can stream interaction prompts (asking the user for input) with no
    // chunks and, if heartbeats pause, nothing else. Interactions are real inbound
    // data and must reset the liveness watchdog, or a live turn would be reaped as
    // a dead pipe. Regression guard for interaction not counting as activity.
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

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // An interaction every 60s for 5 min (well past the 90s window), no heartbeats.
    for (let i = 0; i < 5; i++) {
      act(() => {
        vi.advanceTimersByTime(60 * 1000);
      });
      act(() => {
        stream.emit("interaction", JSON.stringify({ type: "interaction" }));
      });
      expect(result.current.streamError).toBeNull();
      expect(result.current.isStreaming).toBe(true);
    }
  });

  it("native reconnect errors do not keep a dead pipe alive", async () => {
    // A genuinely dead pipe makes EventSource auto-reconnect, dispatching a
    // native (non-MessageEvent) error every few seconds. Those must NOT count as
    // liveness, or the watchdog re-arms forever and the turn hangs — the exact
    // case this backstop exists for. Only real inbound data resets the watchdog.
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

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // Native reconnect errors every 3s across the whole 90s liveness window.
    for (let i = 0; i < 30; i++) {
      act(() => {
        vi.advanceTimersByTime(3 * 1000);
      });
      act(() => {
        stream.emitNativeError();
      });
    }
    act(() => {
      vi.advanceTimersByTime(1000);
    });

    expect(result.current.streamError).toBeTruthy();
    expect(result.current.isStreaming).toBe(false);
    expect(stream.readyState).toBe(MockEventSource.CLOSED);
  });

  it("server error event surfaces its message and ends the turn", async () => {
    // The server is authoritative for termination: an error event ends the turn
    // immediately and shows the server's message, without waiting on the watchdog.
    messagingHandlers();

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: null }),
      { wrapper },
    );

    await act(async () => {
      await result.current.sendMessage("hello");
    });

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    act(() => {
      stream.emit(
        "error",
        JSON.stringify({
          type: "error",
          message: "The agent disconnected. You can try sending again.",
          retryable: true,
        }),
      );
    });

    await waitFor(() => {
      expect(result.current.isStreaming).toBe(false);
    });
    expect(result.current.streamError).toBe(
      "The agent disconnected. You can try sending again.",
    );
  });

  it("a malformed server error event still terminates the turn", async () => {
    // A server-sent error event is a terminal signal even if its payload doesn't
    // parse: it must end the turn (with the default message), not merely reset the
    // liveness watchdog and leave the turn hanging until a backstop fires.
    messagingHandlers();

    const { wrapper } = createHookWrapper();
    const { result } = renderHook(
      () => useDeploymentChat(deploymentId, { conversationId: null }),
      { wrapper },
    );

    await act(async () => {
      await result.current.sendMessage("hello");
    });

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    act(() => {
      stream.emit("error", "not-json{");
    });

    await waitFor(() => {
      expect(result.current.isStreaming).toBe(false);
    });
    expect(result.current.streamError).toBe(
      "The agent stopped responding. You can try sending again.",
    );
  });

  it("content-stall cap reaps a turn that only heartbeats and never produces content", async () => {
    // Defense-in-depth: a sidecar that keeps heartbeating but produces no content
    // and never sends a terminal event resets the liveness watchdog forever.
    // Heartbeats are not content, so the 15-min content-stall cap still ends the
    // turn — the composer can't stay blocked indefinitely against a regressed server.
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

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // Heartbeat every 30s for 14.5 min; the 90s liveness watchdog never fires and
    // heartbeats never reset the content-stall cap.
    for (let i = 0; i < 29; i++) {
      act(() => {
        vi.advanceTimersByTime(30 * 1000);
      });
      act(() => {
        stream.emit("heartbeat", JSON.stringify({ type: "heartbeat" }));
      });
    }
    expect(result.current.isStreaming).toBe(true);
    expect(result.current.streamError).toBeNull();

    // Cross the 15-min content-stall cap.
    act(() => {
      vi.advanceTimersByTime(31 * 1000);
    });

    expect(result.current.streamError).toBeTruthy();
    expect(result.current.isStreaming).toBe(false);
    expect(stream.readyState).toBe(MockEventSource.CLOSED);
  });

  it("a continuously-streaming turn is not cut off by the content-stall cap", async () => {
    // The cap resets on content, so a healthy long turn that keeps streaming
    // chunks (e.g. deep research, large codegen) runs well past 15 min without
    // being cut off — the failure class this change avoids reintroducing.
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

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // A content chunk every 60s for 20 min (past the 15-min cap). 60s is under the
    // 90s liveness window (chunks reset that too), and each chunk resets the stall
    // cap, so the turn keeps streaming with no error.
    for (let i = 0; i < 20; i++) {
      act(() => {
        vi.advanceTimersByTime(60 * 1000);
      });
      act(() => {
        stream.emit(
          "chunk",
          JSON.stringify({ type: "chunk", content: `tok${i} ` }),
        );
      });
      expect(result.current.streamError).toBeNull();
      expect(result.current.isStreaming).toBe(true);
      expect(stream.readyState).not.toBe(MockEventSource.CLOSED);
    }
  });

  it("a pending interaction pauses the content-stall cap so a long human pause isn't reaped", async () => {
    // The agent streams an interaction prompt then waits for the user. Heartbeats
    // keep the liveness watchdog alive, but the content-stall cap must be paused
    // (the turn is parked on the user, not the agent), so a >15-min pause to answer
    // isn't reaped as "stopped producing output".
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

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // The agent asks for input, then goes quiet awaiting the user's answer.
    act(() => {
      stream.emit(
        "interaction",
        JSON.stringify({
          type: "interaction",
          id: "int-1",
          dataSchema: { type: "object" },
          actions: ["submit"],
        }),
      );
    });

    // 16 min pass with only heartbeats (past the 15-min stall cap) while the user
    // is still deciding. The turn must stay alive.
    for (let i = 0; i < 33; i++) {
      act(() => {
        vi.advanceTimersByTime(30 * 1000);
      });
      act(() => {
        stream.emit("heartbeat", JSON.stringify({ type: "heartbeat" }));
      });
      expect(result.current.streamError).toBeNull();
      expect(result.current.isStreaming).toBe(true);
    }
  });

  it("re-arms the content-stall cap after an interaction is resolved", async () => {
    // Resolving an interaction re-arms the paused stall cap, so if the agent then
    // resumes with only heartbeats and no content (a regressed sidecar), the turn
    // is still bounded rather than pinned open forever.
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

    const stream = MockEventSource.instances[0];
    expect(result.current.isStreaming).toBe(true);

    // The agent asks for input (pausing the stall cap); the user answers.
    act(() => {
      stream.emit(
        "interaction",
        JSON.stringify({
          type: "interaction",
          id: "int-1",
          dataSchema: { type: "object" },
          actions: ["submit"],
        }),
      );
    });
    act(() => {
      result.current.clearPendingInteraction();
    });

    // The agent resumes but produces only heartbeats (no content) for 16 min. The
    // re-armed stall cap must still reap it.
    for (let i = 0; i < 32; i++) {
      act(() => {
        vi.advanceTimersByTime(30 * 1000);
      });
      act(() => {
        stream.emit("heartbeat", JSON.stringify({ type: "heartbeat" }));
      });
    }
    act(() => {
      vi.advanceTimersByTime(60 * 1000);
    });

    expect(result.current.streamError).toBeTruthy();
    expect(result.current.isStreaming).toBe(false);
    expect(stream.readyState).toBe(MockEventSource.CLOSED);
  });

  it("surfaces a background conversation's terminal error when the user returns", async () => {
    const convA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
    const convB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
    server.use(
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convA}`,
        () =>
          HttpResponse.json({
            conversation_id: convA,
            messages: [{ id: "ua", role: "user", content: "hi" }],
            assistant_streaming: true,
          }),
      ),
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convB}`,
        () =>
          HttpResponse.json({
            conversation_id: convB,
            messages: [{ id: "ub", role: "user", content: "yo" }],
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

    await waitFor(() => expect(result.current.isStreaming).toBe(true));
    const aStream = MockEventSource.latest();

    // Leave A for B; A's stream stays open in the background.
    rerender({ cid: convB });
    await waitFor(() => expect(result.current.isStreaming).toBe(false));

    // A's turn errors while off-screen — it must not surface on B...
    act(() => {
      aStream.emit(
        "error",
        JSON.stringify({ type: "error", message: "The agent hit an error." }),
      );
    });
    expect(result.current.streamError).toBeNull();

    // ...but returning to A shows it instead of a silently re-armed composer.
    rerender({ cid: convA });
    await waitFor(() =>
      expect(result.current.streamError).toBe("The agent hit an error."),
    );
  });

  it("surfaces a background error when returning from a null 'new chat' state", async () => {
    const convA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
    server.use(
      http.get(
        `/api/v1/deployments/${deploymentId}/chat/conversations/${convA}`,
        () =>
          HttpResponse.json({
            conversation_id: convA,
            messages: [{ id: "ua", role: "user", content: "hi" }],
            assistant_streaming: true,
          }),
      ),
    );

    const { wrapper } = createHookWrapper();
    const { result, rerender } = renderHook(
      ({ cid }: { cid: string | null }) =>
        useDeploymentChat(deploymentId, { conversationId: cid }),
      { wrapper, initialProps: { cid: convA as string | null } },
    );

    await waitFor(() => expect(result.current.isStreaming).toBe(true));
    const aStream = MockEventSource.latest();

    // Switch to a null "new chat"; A's stream stays open in the background.
    rerender({ cid: null });
    await waitFor(() => expect(result.current.isStreaming).toBe(false));

    // A errors while off-screen.
    act(() => {
      aStream.emit(
        "error",
        JSON.stringify({ type: "error", message: "A failed while away." }),
      );
    });
    expect(result.current.streamError).toBeNull();

    // Returning to A from the null state must still surface the stashed error
    // (this null -> conversation transition previously short-circuited it).
    rerender({ cid: convA });
    await waitFor(() =>
      expect(result.current.streamError).toBe("A failed while away."),
    );
  });
});
