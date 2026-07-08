import { useCallback, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useDeploymentChatConversations } from "@/api/queries/chat";
import { chatKeys } from "@/api/queries/keys";
import type { ChatSession } from "@/lib/chat/types";

const DEFAULT_TITLE = "New conversation";

/**
 * Server-backed chat session list. astro-server does not persist chat — it
 * authenticates and forwards to the deployment's messaging sidecar, which owns
 * conversation metadata and message bodies in a deployment-local SQLite store on
 * the agent's shared persistent disk. Keyed by the opaque WorkOS user id.
 */
export function useChatSessions(deploymentId: string) {
  const { data, isLoading } = useDeploymentChatConversations(deploymentId);
  const queryClient = useQueryClient();

  const sessions = useMemo(
    (): ChatSession[] =>
      (data?.conversations ?? []).map((conv) => ({
        conversationId: conv.conversation_id,
        deploymentId,
        title: conv.title?.trim() || DEFAULT_TITLE,
        updatedAt: conv.updated_at,
        assistantStreaming: conv.assistant_streaming,
      })),
    [data?.conversations, deploymentId],
  );

  // Called after each send. The sidecar creates the conversation row and derives
  // its title on the first send (EnsureForSend) and bumps recency thereafter, so
  // the client no longer writes any of that — it just refreshes the list so the
  // new conversation and its server-derived title appear in the history dropdown.
  const recordFirstMessage = useCallback(async () => {
    await queryClient.invalidateQueries({
      queryKey: chatKeys.conversations(deploymentId),
    });
  }, [deploymentId, queryClient]);

  return { sessions, recordFirstMessage, isLoading };
}
