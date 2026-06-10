import type { ApiClient } from "@/lib/api";

/** History poll cadence while a turn is in flight (server persists ~every 500ms). */
export const CHAT_POLL_MS = 500;

export type MessagingStreamHandlers = {
  onChunk: () => void;
  onFinish: () => void;
  onProtocolError: () => void;
};

/**
 * Open the messaging SSE stream for an in-flight assistant turn.
 *
 * Active sends attach a consumer so the proxy keeps persisting chunks. On
 * navigation back to an in-flight thread the server may already be persisting
 * via a detached consumer from the prior connection — polling history is
 * enough for UI catch-up, so callers pass `withStream: false` for recovery.
 */
export function openMessagingStream(
  api: ApiClient,
  deploymentId: string,
  conversationId: string,
  handlers: MessagingStreamHandlers,
): EventSource {
  const es = new EventSource(api.messagingStreamPath(deploymentId, conversationId));
  es.addEventListener("chunk", handlers.onChunk);
  es.addEventListener("finish", handlers.onFinish);
  es.addEventListener("error", (e) => {
    if (e instanceof MessageEvent) handlers.onProtocolError();
  });
  return es;
}
