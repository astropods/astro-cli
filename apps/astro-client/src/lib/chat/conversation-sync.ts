import type {
  ChatAttachment,
  DeploymentChatMessageRecord,
  GetDeploymentChatConversationResponse,
} from "@/lib/api";

/** Initial history page when opening a conversation. */
export const CHAT_INITIAL_PAGE_LIMIT = 100;

/** Tail page size for live refresh while a turn is in flight (poll / SSE chunk). */
export const CHAT_LIVE_TAIL_LIMIT = 32;

function conversationMessages(
  thread: GetDeploymentChatConversationResponse,
): DeploymentChatMessageRecord[] {
  return thread.messages ?? [];
}

/**
 * Client-generated optimistic/streaming row id. sendMessage uses `user-<ts>` for
 * the optimistic user row and patchConversationAssistantChunk uses
 * `assistant-<ts>` for the streaming assistant row before the server id lands.
 * Server ids are UUIDs, which never match this shape.
 */
function isClientTempId(id: string): boolean {
  return /^(assistant|user)-\d+$/.test(id);
}

/**
 * Merge a tail fetch into cached history. Overlap is matched by message id so
 * streaming assistant rows update in place without re-downloading the thread.
 */
export function mergeConversationTail(
  existing: GetDeploymentChatConversationResponse,
  tail: GetDeploymentChatConversationResponse,
): GetDeploymentChatConversationResponse {
  const tailMessages = conversationMessages(tail);
  if (tailMessages.length === 0) {
    return {
      ...existing,
      assistant_streaming: tail.assistant_streaming,
      updated_at: tail.updated_at,
      title: tail.title,
    };
  }

  const existingMessages = conversationMessages(existing);
  const tailIds = new Set(tailMessages.map((m) => m.id));
  const overlapIdx = existingMessages.findIndex((m) => tailIds.has(m.id));
  // Keep the history that precedes the authoritative tail window, but drop any
  // client-temporary rows (optimistic user / streaming assistant ids). The tail
  // supersedes them, and keeping a stale streaming row would leave an orphan
  // partial assistant bubble after a mid-stream reconnect or conversation switch.
  const keptPrefix = (
    overlapIdx >= 0
      ? existingMessages.slice(0, overlapIdx)
      : existingMessages.filter((m) => !tailIds.has(m.id))
  ).filter((m) => !isClientTempId(m.id));
  const mergedMessages = [...keptPrefix, ...tailMessages];

  return {
    conversation_id: tail.conversation_id,
    title: tail.title,
    updated_at: tail.updated_at,
    assistant_streaming: tail.assistant_streaming,
    messages: mergedMessages,
    has_more: keptPrefix.length > 0 || !!existing.has_more || !!tail.has_more,
    oldest_seq:
      keptPrefix.length > 0 ? existing.oldest_seq : tail.oldest_seq,
  };
}

function emptyConversation(
  conversationId: string,
): GetDeploymentChatConversationResponse {
  return {
    conversation_id: conversationId,
    title: "",
    updated_at: new Date().toISOString(),
    messages: [],
  };
}

/** Optimistically append a user message while the send request is in flight. */
export function patchConversationUserMessage(
  existing: GetDeploymentChatConversationResponse | undefined,
  conversationId: string,
  message: DeploymentChatMessageRecord,
): GetDeploymentChatConversationResponse {
  const base = existing ?? emptyConversation(conversationId);
  const messages = conversationMessages(base);
  return {
    ...base,
    assistant_streaming: true,
    messages: [...messages, message],
  };
}

/** Apply one SSE assistant chunk to the cached thread (single client-side source
 *  of truth). Attachments arrive only on the END chunk; when present they are set
 *  on the assistant row so agent-produced files render as download chips. */
export function patchConversationAssistantChunk(
  existing: GetDeploymentChatConversationResponse,
  assistantId: string,
  content: string,
  chunkType?: string,
  attachments?: ChatAttachment[],
): GetDeploymentChatConversationResponse {
  const messages = [...conversationMessages(existing)];
  const idx = messages.findIndex((m) => m.id === assistantId);
  const withAttachments =
    attachments && attachments.length > 0 ? { attachments } : {};

  if (chunkType === "replace" || idx < 0) {
    const row: DeploymentChatMessageRecord = {
      id: assistantId,
      role: "assistant",
      content,
      ...withAttachments,
    };
    if (idx >= 0) {
      messages[idx] = row;
    } else {
      messages.push(row);
    }
  } else {
    messages[idx] = {
      ...messages[idx],
      content: messages[idx].content + content,
      ...withAttachments,
    };
  }

  return {
    ...existing,
    assistant_streaming: true,
    messages,
  };
}

/** Roll back an optimistic user row after a failed send. */
export function removeConversationMessage(
  existing: GetDeploymentChatConversationResponse,
  messageId: string,
): GetDeploymentChatConversationResponse {
  return {
    ...existing,
    assistant_streaming: false,
    messages: conversationMessages(existing).filter((m) => m.id !== messageId),
  };
}

/** Prepend an older page fetched via before_seq. */
export function mergeConversationOlder(
  existing: GetDeploymentChatConversationResponse,
  older: GetDeploymentChatConversationResponse,
): GetDeploymentChatConversationResponse {
  return {
    conversation_id: existing.conversation_id,
    title: existing.title,
    updated_at: existing.updated_at,
    assistant_streaming: existing.assistant_streaming,
    messages: [...conversationMessages(older), ...conversationMessages(existing)],
    has_more: older.has_more,
    oldest_seq: older.oldest_seq,
  };
}
