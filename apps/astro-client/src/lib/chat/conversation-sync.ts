import type { GetDeploymentChatConversationResponse } from "@/lib/api";

/** Initial history page when opening a conversation. */
export const CHAT_INITIAL_PAGE_LIMIT = 100;

/** Tail page size for live refresh while a turn is in flight (poll / SSE chunk). */
export const CHAT_LIVE_TAIL_LIMIT = 32;

/**
 * Merge a tail fetch into cached history. Overlap is matched by message id so
 * streaming assistant rows update in place without re-downloading the thread.
 */
export function mergeConversationTail(
  existing: GetDeploymentChatConversationResponse,
  tail: GetDeploymentChatConversationResponse,
): GetDeploymentChatConversationResponse {
  if (tail.messages.length === 0) {
    return {
      ...existing,
      assistant_streaming: tail.assistant_streaming,
      updated_at: tail.updated_at,
      title: tail.title,
    };
  }

  const tailIds = new Set(tail.messages.map((m) => m.id));
  const prefix = existing.messages.filter((m) => !tailIds.has(m.id));
  const overlapIdx = existing.messages.findIndex((m) => tailIds.has(m.id));
  const mergedMessages =
    overlapIdx >= 0
      ? [...existing.messages.slice(0, overlapIdx), ...tail.messages]
      : [...prefix, ...tail.messages];

  const keptPrefix = overlapIdx >= 0 ? overlapIdx : prefix.length;

  return {
    conversation_id: tail.conversation_id,
    title: tail.title,
    updated_at: tail.updated_at,
    assistant_streaming: tail.assistant_streaming,
    messages: mergedMessages,
    has_more: keptPrefix > 0 || !!existing.has_more || !!tail.has_more,
    oldest_seq:
      keptPrefix > 0 ? existing.oldest_seq : tail.oldest_seq,
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
    messages: [...older.messages, ...existing.messages],
    has_more: older.has_more,
    oldest_seq: older.oldest_seq,
  };
}
