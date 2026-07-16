import type { ThreadMessageLike } from "@assistant-ui/react";
import type { ChatAttachment } from "@/lib/api";
import type { ChatMessage } from "@/lib/chat/message";
import { ASTRO_FILE_PART } from "@/lib/messaging/deployment-attachment-adapter";

function assistantTextContent(m: ChatMessage): string {
  // assistant-ui drops zero-length text parts; keep a placeholder while streaming.
  if (m.isStreaming && !m.content) return " ";
  return m.content;
}

// User-attached files use assistant-ui's native attachment system (attachments
// are only valid on user messages). The files-API reference rides in a `data`
// content part so the rendered chip can resolve key/name/size for download.
function toUserAttachments(
  atts: ChatAttachment[] | undefined,
): ThreadMessageLike["attachments"] {
  if (!atts || atts.length === 0) return undefined;
  return atts.map((a) => ({
    id: a.key,
    type: a.content_type.startsWith("image/") ? "image" : "file",
    name: a.name,
    contentType: a.content_type,
    status: { type: "complete" as const },
    content: [{ type: "data" as const, name: ASTRO_FILE_PART, data: a }],
  }));
}

// Agent-produced files are modeled as assistant content parts (assistant
// messages can't carry `attachments`; content parts are the framework's model
// for assistant output). Rendered by the message part switch in AssistantMessage.
function toFileParts(atts: ChatAttachment[] | undefined) {
  return (atts ?? []).map((a) => ({
    type: "data" as const,
    name: ASTRO_FILE_PART,
    data: a,
  }));
}

export function chatMessagesToThreadMessages(
  messages: ChatMessage[],
): ThreadMessageLike[] {
  return messages.map((m) => {
    if (m.role === "user") {
      const attachments = toUserAttachments(m.attachments);
      return {
        id: m.id,
        role: "user",
        content: [{ type: "text", text: m.content }],
        ...(attachments ? { attachments } : {}),
      };
    }

    const base: ThreadMessageLike = {
      id: m.id,
      role: "assistant",
      content: [
        { type: "text", text: assistantTextContent(m) },
        ...toFileParts(m.attachments),
      ],
    };
    if (m.isStreaming) {
      return {
        ...base,
        status: { type: "running" },
        metadata: { custom: { deploymentChatStreaming: true } },
      };
    }
    return { ...base, status: { type: "complete", reason: "stop" } };
  });
}
