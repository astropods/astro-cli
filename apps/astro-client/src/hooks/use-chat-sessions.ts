import { useCallback, useMemo } from "react";
import {
  useDeploymentChatConversations,
  useUpsertDeploymentChatConversation,
} from "@/api/queries/chat";
import type { ChatSession } from "@/lib/chat/types";

const DEFAULT_TITLE = "New conversation";

export function titleFromFirstMessage(
  preview: string,
  existingTitle?: string,
): string {
  const trimmed = preview.trim().slice(0, 80);
  return trimmed || existingTitle || DEFAULT_TITLE;
}

/**
 * Server-backed chat session list. Conversation metadata (title, recency) is
 * persisted in astro-server (keyed by the opaque user id); message content is
 * hydrated from Postgres (messaging proxy persistence) with Langfuse as primary
 * when traces exist. Survives reloads and is shared across the user's devices.
 */
export function useChatSessions(deploymentId: string) {
  const { data, isLoading } = useDeploymentChatConversations(deploymentId);
  const upsert = useUpsertDeploymentChatConversation(deploymentId);

  const sessions = useMemo(
    (): ChatSession[] =>
      (data?.conversations ?? []).map((conv) => ({
        conversationId: conv.conversation_id,
        deploymentId,
        title: conv.title?.trim() || DEFAULT_TITLE,
        updatedAt: conv.updated_at,
      })),
    [data?.conversations, deploymentId],
  );

  // Called on every send: persists the conversation row, derives a title on the
  // first real message, and bumps recency thereafter. An empty title is a pure
  // "touch" on the server, so it never clobbers an existing/user-set title.
  const recordFirstMessage = useCallback(
    async (convId: string, preview: string) => {
      const existing = sessions.find((s) => s.conversationId === convId);
      const hasTitle = !!existing && existing.title !== DEFAULT_TITLE;
      const title = hasTitle ? "" : titleFromFirstMessage(preview, existing?.title);
      await upsert.mutateAsync({ conversationId: convId, title });
    },
    [sessions, upsert],
  );

  return { sessions, recordFirstMessage, isLoading };
}
