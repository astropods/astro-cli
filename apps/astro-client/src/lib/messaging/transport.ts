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
  // Server-sent terminal error (agent failed / disconnected / stalled).
  onError: (message: string) => void;
  // Any inbound SSE event (chunk, heartbeat, ...): the transport is alive.
  onActivity: () => void;
  onInteraction?: (interaction: Interaction) => void;
  // A server-injected thread row the client didn't send — a resolved-interaction note (role "note") or a "write your own reply" (role "user") — and the boundary that starts the continuation's new bubble.
  onInjected?: (id: string, role: string, content: string) => void;
};

type SsePayload = {
  type?: string;
  chunk_type?: string;
  content?: string;
  attachments?: ChatAttachment[];
  message?: string;
  id?: string;
  role?: string;
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
    if (typ === "injected" && typeof payload.content === "string") {
      handlers.onInjected?.(payload.id ?? "", payload.role ?? "note", payload.content);
      return;
    }
    if (typ === "error") {
      handlers.onError(
        payload.message || "The agent stopped responding. You can try sending again.",
      );
      return;
    }
    if (typ === "finish") {
      handlers.onFinish();
    }
  };

  es.addEventListener("chunk", (e) => {
    handlers.onActivity();
    if (e instanceof MessageEvent) handleData("chunk", String(e.data));
  });
  es.addEventListener("heartbeat", () => handlers.onActivity());
  es.addEventListener("status", () => handlers.onActivity());
  es.addEventListener("interaction", (e) => {
    handlers.onActivity();
    if (e instanceof MessageEvent) handleData("interaction", String(e.data));
  });
  es.addEventListener("injected", (e) => {
    handlers.onActivity();
    if (e instanceof MessageEvent) handleData("injected", String(e.data));
  });
  es.addEventListener("finish", (e) => {
    handlers.onActivity();
    if (e instanceof MessageEvent) handleData("finish", String(e.data));
    else handlers.onFinish();
  });
  es.addEventListener("error", (e) => {
    // A plain Event is a native connection error: EventSource auto-reconnects and
    // the liveness watchdog (loss of heartbeats) covers a dead pipe, so it must
    // NOT count as activity or it would re-arm the watchdog every reconnect. A
    // MessageEvent is a server-sent error event: always terminal, even if its
    // payload is malformed (fall back to the default message).
    if (!(e instanceof MessageEvent)) return;
    const payload = parseSsePayload(String(e.data));
    handlers.onError(
      payload?.message ||
        "The agent stopped responding. You can try sending again.",
    );
  });
  es.onmessage = (e) => {
    handlers.onActivity();
    handleData("", String(e.data));
  };

  return es;
}
