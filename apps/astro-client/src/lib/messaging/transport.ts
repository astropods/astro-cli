import type { ApiClient } from "@/lib/api";

/** Legacy poll cadence for chat query hooks (unused while history is in-session only). */
export const CHAT_POLL_MS = 500;

export type MessagingStreamHandlers = {
  onChunk: (content: string, chunkType?: string) => void;
  onFinish: () => void;
  onProtocolError: () => void;
};

type SsePayload = {
  type?: string;
  chunk_type?: string;
  content?: string;
};

function parseSsePayload(data: string): SsePayload | null {
  try {
    return JSON.parse(data) as SsePayload;
  } catch {
    return null;
  }
}

/**
 * Open the messaging SSE stream for an in-flight assistant turn.
 *
 * TODO: When Langfuse-backed history lands, clients can recover threads from
 * the server instead of relying on in-session SSE accumulation.
 */
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
      handlers.onChunk(payload.content, payload.chunk_type);
      return;
    }
    if (typ === "finish" || typ === "error") {
      handlers.onFinish();
    }
  };

  es.addEventListener("chunk", (e) => {
    if (e instanceof MessageEvent) handleData("chunk", String(e.data));
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
