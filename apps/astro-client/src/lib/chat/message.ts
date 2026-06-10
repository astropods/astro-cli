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
  records: DeploymentChatMessageRecord[],
  streamingMessageId: string | null,
): ChatMessage[] {
  return records.map((m) => ({
    id: m.id,
    role: m.role,
    content: m.content,
    isStreaming: streamingMessageId != null && m.id === streamingMessageId,
  }));
}

export function streamingAssistantMessageId(
  records: DeploymentChatMessageRecord[],
  turnInFlight: boolean,
): string | null {
  if (!turnInFlight) return null;
  const tail = records[records.length - 1];
  return tail?.role === "assistant" ? tail.id : null;
}

/**
 * Server-authoritative "assistant turn in flight": the messaging proxy is
 * persisting a reply, or the trailing user message has no reply yet.
 */
export function serverTurnInFlight(
  thread: GetDeploymentChatConversationResponse,
): boolean {
  if (thread.assistant_streaming) return true;
  return thread.messages.at(-1)?.role === "user";
}
