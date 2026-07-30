import { useCallback, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useDeploymentChatConversations } from "@/api/queries/chat";
import { useDeploymentChatReadiness } from "@/hooks/use-deployment-chat-readiness";
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
  // Only list conversations once the agent is reachable; the list proxies to the
  // sidecar and would 5xx (tripping the per-route alert) against a stopped or
  // unreachable deployment.
  const { ready, resolved } = useDeploymentChatReadiness(deploymentId);
  const { data, isLoading } = useDeploymentChatConversations(deploymentId, ready);
  const queryClient = useQueryClient();

  // A disabled TanStack query reports isLoading:false, so while the readiness
  // gate is still settling the list looks "loaded but empty". Surface that
  // pre-fetch window as loading (until the gate resolves, then track the real
  // query) so run-once consumers like auto-select wait for actual data instead
  // of firing against an empty list. An unreachable agent resolves to not-ready
  // with no fetch, so loading correctly ends without the list ever running.
  const loading = !resolved || (ready && isLoading);

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

  return { sessions, recordFirstMessage, isLoading: loading };
}
