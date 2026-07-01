import type {
  DeploymentChatMessageRecord,
  GetDeploymentChatConversationResponse,
} from "@/lib/api";

export type ChatRole = "user" | "assistant";

/** View-model message for deployment chat UI (server-owned history). */
export type ChatMessage = {
  id: string;
  role: ChatRole;
  content: string;
  isStreaming?: boolean;
};

export function mapServerMessages(
  records: DeploymentChatMessageRecord[] | null | undefined,
  streamingMessageId: string | null,
): ChatMessage[] {
  return (records ?? []).map((m) => ({
    id: m.id,
    role: m.role,
    content: m.content,
    isStreaming: streamingMessageId != null && m.id === streamingMessageId,
  }));
}

/**
 * Server-authoritative "assistant turn in flight": the messaging proxy is
 * persisting a reply (`assistant_streaming`), or an optimistic client patch
 * marked the thread active before the server flag lands.
 */
export function serverTurnInFlight(
  thread: GetDeploymentChatConversationResponse,
): boolean {
  if (thread.assistant_streaming === false) return false;
  if (thread.assistant_streaming) return true;
  const messages = thread.messages ?? [];
  return messages.at(-1)?.role === "user";
}

/** Streaming assistant row id for an in-flight thread (null while awaiting first chunk). */
export function inFlightAssistantMessageId(
  thread: GetDeploymentChatConversationResponse | undefined,
): string | null {
  if (!thread || !serverTurnInFlight(thread)) return null;
  const tail = (thread.messages ?? []).at(-1);
  return tail?.role === "assistant" ? tail.id : null;
}

/**
 * Whether a turn should be treated as in flight, reconciling the optimistic
 * local turn with what the server reports.
 *
 * `activeLocalTurn` (we just sent and the SSE is still open) is authoritative:
 * a freshly-sent turn must stay in flight even if an early history snapshot says
 * otherwise. On a new conversation the history GET can resolve before the server
 * has registered the assistant turn — Langfuse write→read lag and first-token
 * latency leave a window where `assistant_streaming` is false — and without this,
 * that stale "not in flight" snapshot tears down the live stream (the loading dot
 * vanishes and the reply is lost until reload). The turn ends through the SSE
 * (finish/error) or the in-flight timeout, both of which clear `activeLocalTurn`,
 * after which the server snapshot is trusted again.
 */
export function deriveTurnInFlight(params: {
  activeLocalTurn: boolean;
  serverThread: GetDeploymentChatConversationResponse | undefined;
  cachedThread: GetDeploymentChatConversationResponse | undefined;
  isStreaming: boolean;
}): boolean {
  if (params.activeLocalTurn) return true;
  if (params.serverThread) return serverTurnInFlight(params.serverThread);
  if (params.cachedThread) return serverTurnInFlight(params.cachedThread);
  return params.isStreaming;
}

