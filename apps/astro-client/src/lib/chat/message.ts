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

