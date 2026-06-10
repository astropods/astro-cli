import { useCallback, useMemo } from "react";
import {
  useDeploymentChatConversations,
  useUpsertDeploymentChatConversation,
} from "@/api/queries/chat";
import type { ChatSession } from "@/lib/chat/types";

export function titleFromFirstMessage(
  preview: string,
  existingTitle?: string,
): string {
  const trimmed = preview.trim().slice(0, 80);
  return trimmed || existingTitle || "New conversation";
}

export function useChatSessions(deploymentId: string) {
  const { data, isLoading } = useDeploymentChatConversations(deploymentId);
  const upsert = useUpsertDeploymentChatConversation(deploymentId);

  const sessions = useMemo((): ChatSession[] => {
    return (data?.conversations ?? []).map((c) => ({
      conversationId: c.conversation_id,
      deploymentId,
      title: c.title,
      updatedAt: c.updated_at,
    }));
  }, [data?.conversations, deploymentId]);

  const recordSession = useCallback(
    (session: ChatSession) => {
      upsert.mutate({
        conversationId: session.conversationId,
        title: session.title,
      });
    },
    [upsert],
  );

  const recordFirstMessage = useCallback(
    (convId: string, preview: string) => {
      const existing = sessions.find((s) => s.conversationId === convId);
      recordSession({
        conversationId: convId,
        deploymentId,
        title: titleFromFirstMessage(preview, existing?.title),
        updatedAt: new Date().toISOString(),
      });
    },
    [deploymentId, recordSession, sessions],
  );

  return { sessions, recordSession, recordFirstMessage, isLoading };
}
