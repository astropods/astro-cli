import { describe, expect, it } from "vitest";
import type { ChatMessage } from "@/lib/chat/message";
import { chatMessagesToThreadMessages } from "./chat-message-adapter";

describe("chatMessagesToThreadMessages", () => {
  it("maps a note to a system message with exactly one text part", () => {
    const messages: ChatMessage[] = [
      { id: "n1", role: "note", content: "Approved" },
    ];
    const [tm] = chatMessagesToThreadMessages(messages);
    expect(tm.role).toBe("system");
    // assistant-ui rejects a system message that isn't exactly one text part.
    expect(tm.content).toEqual([{ type: "text", text: "Approved" }]);
  });

  it("keeps user and assistant roles distinct from a note", () => {
    const [user, assistant] = chatMessagesToThreadMessages([
      { id: "u1", role: "user", content: "hi" },
      { id: "a1", role: "assistant", content: "hello" },
    ]);
    expect(user.role).toBe("user");
    expect(assistant.role).toBe("assistant");
  });
});
