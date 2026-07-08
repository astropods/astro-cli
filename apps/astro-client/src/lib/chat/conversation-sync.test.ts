import { describe, expect, it } from "vitest";
import type { GetDeploymentChatConversationResponse } from "@/lib/api";
import {
  mergeConversationTail,
  patchConversationAssistantChunk,
  patchConversationUserMessage,
  removeConversationMessage,
} from "./conversation-sync";

const convId = "34ac809f-9a55-4b57-b92e-00020720c700";

function thread(
  messages: GetDeploymentChatConversationResponse["messages"],
  updatedAt = "1",
): GetDeploymentChatConversationResponse {
  return {
    conversation_id: convId,
    title: "t",
    updated_at: updatedAt,
    messages,
  };
}

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

describe("mergeConversationTail", () => {
  it("drops a stale client-temp streaming row superseded by the authoritative tail", () => {
    // Mid-stream reconnect: the cache still holds a temp streaming assistant row
    // (a fragment), while the tail brings the real server rows. The temp row must
    // not survive as an orphan bubble alongside the real assistant reply.
    const existing = thread([
      { id: "srv-user-1", role: "user", content: "q" },
      { id: "assistant-1699999999999", role: "assistant", content: "frag" },
    ]);
    const tail = thread(
      [
        { id: "srv-user-1", role: "user", content: "q" },
        { id: "srv-asst-1", role: "assistant", content: "full reply" },
      ],
      "2",
    );

    const merged = mergeConversationTail(existing, tail);
    expect(merged.messages.map((m) => m.id)).toEqual([
      "srv-user-1",
      "srv-asst-1",
    ]);
  });

  it("keeps real history that precedes the tail window", () => {
    const existing = thread([
      { id: "srv-old-1", role: "user", content: "old q" },
      { id: "srv-old-2", role: "assistant", content: "old a" },
      { id: "srv-user-2", role: "user", content: "new q" },
    ]);
    const tail = thread(
      [
        { id: "srv-user-2", role: "user", content: "new q" },
        { id: "srv-asst-2", role: "assistant", content: "new a" },
      ],
      "2",
    );

    const merged = mergeConversationTail(existing, tail);
    expect(merged.messages.map((m) => m.id)).toEqual([
      "srv-old-1",
      "srv-old-2",
      "srv-user-2",
      "srv-asst-2",
    ]);
  });
});
