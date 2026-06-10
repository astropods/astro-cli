import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { createHookWrapper } from "@/test/test-utils";
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
});
