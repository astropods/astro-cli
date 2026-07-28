import type { ApiClient, ChatAttachment } from "@/lib/api";
import { parseInteraction, type Interaction } from "@/lib/chat/interaction";

/** Poll cadence while a turn is in flight without an active SSE session (e.g. after reload). */
export const CHAT_POLL_MS = 500;

export type MessagingStreamHandlers = {
  onChunk: (
    content: string,
    chunkType?: string,
    attachments?: ChatAttachment[],
  ) => void;
  onFinish: () => void;
  onProtocolError: () => void;
  onInteraction?: (interaction: Interaction) => void;
};

type SsePayload = {
  type?: string;
  chunk_type?: string;
  content?: string;
  attachments?: ChatAttachment[];
};

function parseSsePayload(data: string): SsePayload | null {
  try {
    return JSON.parse(data) as SsePayload;
  } catch {
    return null;
  }
}

/** Open the messaging SSE stream; chunks patch the TanStack conversation cache. */
export function openMessagingStream(
  api: ApiClient,
  deploymentId: string,
  conversationId: string,
  handlers: MessagingStreamHandlers,
): EventSource {
  const es = new EventSource(api.messagingStreamPath(deploymentId, conversationId));

  const handleData = (eventName: string, data: string) => {
    const payload = parseSsePayload(data);
    if (!payload) return;
    const typ = payload.type ?? eventName;
    if (typ === "chunk" && typeof payload.content === "string") {
      handlers.onChunk(payload.content, payload.chunk_type, payload.attachments);
      return;
    }
    if (typ === "interaction") {
      const interaction = parseInteraction(payload);
      if (interaction) handlers.onInteraction?.(interaction);
      return;
    }
    if (typ === "finish" || typ === "error") {
      handlers.onFinish();
    }
  };

  es.addEventListener("chunk", (e) => {
    if (e instanceof MessageEvent) handleData("chunk", String(e.data));
  });
  es.addEventListener("interaction", (e) => {
    if (e instanceof MessageEvent) handleData("interaction", String(e.data));
  });
  es.addEventListener("finish", (e) => {
    if (e instanceof MessageEvent) handleData("finish", String(e.data));
    else handlers.onFinish();
  });
  es.addEventListener("error", (e) => {
    if (e instanceof MessageEvent) handlers.onProtocolError();
  });
  es.onmessage = (e) => handleData("", String(e.data));

  return es;
}
