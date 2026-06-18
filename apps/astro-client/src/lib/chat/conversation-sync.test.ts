import { describe, expect, it } from "vitest";
import {
  patchConversationAssistantChunk,
  patchConversationUserMessage,
  removeConversationMessage,
} from "./conversation-sync";

const convId = "34ac809f-9a55-4b57-b92e-00020720c700";

describe("patchConversationUserMessage", () => {
  it("appends an optimistic user row and marks streaming", () => {
    const patched = patchConversationUserMessage(undefined, convId, {
      id: "user-1",
      role: "user",
      content: "hello",
    });
    expect(patched.messages).toEqual([
      { id: "user-1", role: "user", content: "hello" },
    ]);
    expect(patched.assistant_streaming).toBe(true);
  });
});

describe("patchConversationAssistantChunk", () => {
  it("appends assistant on first chunk", () => {
    const base = patchConversationUserMessage(undefined, convId, {
      id: "user-1",
      role: "user",
      content: "hello",
    });
    const patched = patchConversationAssistantChunk(
      base,
      "assistant-1",
      "hi",
    );
    expect(patched.messages).toHaveLength(2);
    expect(patched.messages[1]).toEqual({
      id: "assistant-1",
      role: "assistant",
      content: "hi",
    });
  });

  it("accumulates incremental chunks", () => {
    const base = patchConversationAssistantChunk(
      patchConversationUserMessage(undefined, convId, {
        id: "user-1",
        role: "user",
        content: "hello",
      }),
      "assistant-1",
      "hi",
    );
    const patched = patchConversationAssistantChunk(
      base,
      "assistant-1",
      " there",
    );
    expect(patched.messages[1].content).toBe("hi there");
  });
});

describe("removeConversationMessage", () => {
  it("rolls back a failed optimistic user send", () => {
    const base = patchConversationUserMessage(undefined, convId, {
      id: "user-1",
      role: "user",
      content: "hello",
    });
    const rolled = removeConversationMessage(base, "user-1");
    expect(rolled.messages).toEqual([]);
    expect(rolled.assistant_streaming).toBe(false);
  });
});
