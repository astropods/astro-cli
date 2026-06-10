import type { ThreadMessageLike } from "@assistant-ui/react";
import type { ChatMessage } from "@/lib/chat/message";

function assistantTextContent(m: ChatMessage): string {
  // assistant-ui drops zero-length text parts; keep a placeholder while streaming.
  if (m.isStreaming && !m.content) return " ";
  return m.content;
}

export function chatMessagesToThreadMessages(
  messages: ChatMessage[],
): ThreadMessageLike[] {
  return messages.map((m) => {
    const text =
      m.role === "assistant" ? assistantTextContent(m) : m.content;
    const base: ThreadMessageLike = {
      id: m.id,
      role: m.role,
      content: [{ type: "text", text }],
    };
    if (m.role === "assistant") {
      if (m.isStreaming) {
        return {
          ...base,
          status: { type: "running" },
          metadata: { custom: { deploymentChatStreaming: true } },
        };
      }
      return { ...base, status: { type: "complete", reason: "stop" } };
    }
    return base;
  });
}
