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

function sameChatMessage(a: ChatMessage, b: ChatMessage): boolean {
  return a.role === b.role && a.content.trim() === b.content.trim();
}

/**
 * Merges persisted server history with optimistic local turns. Streaming
 * assistant rows are kept at the tail; confirmed locals are matched against the
 * server suffix even when a streaming assistant sits above them in local state.
 */
export function mergeLocalAndServerMessages(
  serverMessages: ChatMessage[],
  localMessages: ChatMessage[],
): ChatMessage[] {
  if (serverMessages.length === 0) return localMessages;
  if (localMessages.length === 0) return serverMessages;

  let localIdx = localMessages.length - 1;
  let serverIdx = serverMessages.length - 1;
  const streamingTail: ChatMessage[] = [];

  while (localIdx >= 0 && localMessages[localIdx].isStreaming) {
    streamingTail.unshift(localMessages[localIdx]);
    localIdx--;
  }

  while (localIdx >= 0 && serverIdx >= 0) {
    if (!sameChatMessage(localMessages[localIdx], serverMessages[serverIdx])) {
      break;
    }
    localIdx--;
    serverIdx--;
  }

  const pendingLocal = [
    ...localMessages.slice(0, localIdx + 1),
    ...streamingTail,
  ];
  return [...serverMessages, ...pendingLocal];
}
