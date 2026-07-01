import { describe, expect, it } from "vitest";
import type { GetDeploymentChatConversationResponse } from "@/lib/api";
import {
  deriveTurnInFlight,
  serverTurnInFlight,
  inFlightAssistantMessageId,
} from "./message";

const thread = (
  overrides: Partial<GetDeploymentChatConversationResponse>,
): GetDeploymentChatConversationResponse => ({
  conversation_id: "c1",
  title: "",
  updated_at: "",
  messages: [],
  ...overrides,
});

describe("serverTurnInFlight", () => {
  it("returns false when messages is null", () => {
    expect(
      serverTurnInFlight({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        messages: null as unknown as [],
      }),
    ).toBe(false);
  });

  it("returns true when trailing message is user and stream is active", () => {
    expect(
      serverTurnInFlight({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: true,
        messages: [{ id: "u1", role: "user", content: "hi" }],
      }),
    ).toBe(true);
  });

  it("returns false when server cleared assistant_streaming despite user tail", () => {
    expect(
      serverTurnInFlight({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: false,
        messages: [
          { id: "a1", role: "assistant", content: "done" },
          { id: "u1", role: "user", content: "stuck" },
        ],
      }),
    ).toBe(false);
  });
});

describe("deriveTurnInFlight", () => {
  it("keeps a just-sent turn in flight despite an early not-in-flight server snapshot", () => {
    // Regression: on a new chat the history GET can resolve with
    // assistant_streaming=false before the server registers the assistant turn.
    // While our local SSE is open, that stale snapshot must not end the turn.
    expect(
      deriveTurnInFlight({
        activeLocalTurn: true,
        serverThread: thread({
          assistant_streaming: false,
          messages: [{ id: "u1", role: "user", content: "hi" }],
        }),
        cachedThread: undefined,
        isStreaming: true,
      }),
    ).toBe(true);
  });

  it("trusts the server snapshot once no local SSE is active", () => {
    expect(
      deriveTurnInFlight({
        activeLocalTurn: false,
        serverThread: thread({
          assistant_streaming: false,
          messages: [{ id: "u1", role: "user", content: "hi" }],
        }),
        cachedThread: undefined,
        isStreaming: true,
      }),
    ).toBe(false);

    expect(
      deriveTurnInFlight({
        activeLocalTurn: false,
        serverThread: thread({ assistant_streaming: true, messages: [] }),
        cachedThread: undefined,
        isStreaming: false,
      }),
    ).toBe(true);
  });

  it("falls back to the cached thread, then isStreaming, when there is no server data", () => {
    expect(
      deriveTurnInFlight({
        activeLocalTurn: false,
        serverThread: undefined,
        cachedThread: thread({
          messages: [{ id: "u1", role: "user", content: "hi" }],
        }),
        isStreaming: false,
      }),
    ).toBe(true); // user-tail cached thread reads as in flight

    expect(
      deriveTurnInFlight({
        activeLocalTurn: false,
        serverThread: undefined,
        cachedThread: undefined,
        isStreaming: true,
      }),
    ).toBe(true);
  });
});

describe("inFlightAssistantMessageId", () => {
  it("returns null while awaiting the first assistant chunk", () => {
    expect(
      inFlightAssistantMessageId({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: true,
        messages: [{ id: "u1", role: "user", content: "hi" }],
      }),
    ).toBeNull();
  });

  it("returns the tail assistant id while streaming", () => {
    expect(
      inFlightAssistantMessageId({
        conversation_id: "c1",
        title: "",
        updated_at: "",
        assistant_streaming: true,
        messages: [
          { id: "u1", role: "user", content: "hi" },
          { id: "a1", role: "assistant", content: "hello" },
        ],
      }),
    ).toBe("a1");
  });
});
