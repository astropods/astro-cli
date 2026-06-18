/** TanStack Query bindings for GET/PUT/POST /deployments/:id/chat (web /chat UI). */
import type { MutableRefObject } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";
import {
  type ApiClient,
  type GetDeploymentChatConversationResponse,
} from "@/lib/api";
import { useApiClient } from "@/lib/api-context";
import {
  CHAT_INITIAL_PAGE_LIMIT,
  CHAT_LIVE_TAIL_LIMIT,
  mergeConversationTail,
} from "@/lib/chat/conversation-sync";
import { CHAT_POLL_MS } from "@/lib/messaging/transport";
import { chatKeys } from "./keys";

export async function fetchDeploymentChatConversation(
  api: ApiClient,
  deploymentId: string,
  conversationId: string,
): Promise<GetDeploymentChatConversationResponse> {
  return api.getDeploymentChatConversation(deploymentId, conversationId, {
    limit: CHAT_INITIAL_PAGE_LIMIT,
  });
}

/** Tail-only refresh for live turns — merges into the cached full thread. */
export async function refreshDeploymentChatTail(
  queryClient: QueryClient,
  api: ApiClient,
  deploymentId: string,
  conversationId: string,
): Promise<GetDeploymentChatConversationResponse | undefined> {
  const key = chatKeys.conversation(deploymentId, conversationId);
  const existing =
    queryClient.getQueryData<GetDeploymentChatConversationResponse>(key);
  if (!existing || (existing.messages ?? []).length === 0) return existing;

  const tail = await api.getDeploymentChatConversation(
    deploymentId,
    conversationId,
    { limit: CHAT_LIVE_TAIL_LIMIT },
  );
  const merged = mergeConversationTail(existing, tail);
  queryClient.setQueryData(key, merged);
  return merged;
}

export function useDeploymentChatConversations(deploymentId: string) {
  const api = useApiClient();
  return useQuery({
    queryKey: chatKeys.conversations(deploymentId),
    queryFn: () => api.listDeploymentChatConversations(deploymentId),
    enabled: !!deploymentId,
  });
}

export function useDeploymentChatConversation(
  deploymentId: string,
  conversationId: string | null | undefined,
  options?: {
    shouldPoll?: (data: GetDeploymentChatConversationResponse | undefined) => boolean;
    /** When true, poll refetches only the message tail and merges into cache. */
    useTailPollRef?: MutableRefObject<boolean>;
  },
) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  const key = chatKeys.conversation(deploymentId, conversationId ?? "");

  return useQuery({
    queryKey: key,
    queryFn: async () => {
      if (options?.useTailPollRef?.current) {
        const merged = await refreshDeploymentChatTail(
          queryClient,
          api,
          deploymentId,
          conversationId!,
        );
        if (merged) return merged;
      }
      return fetchDeploymentChatConversation(api, deploymentId, conversationId!);
    },
    enabled: !!deploymentId && !!conversationId,
    staleTime: 0,
    refetchOnMount: "always",
    refetchInterval: (query) =>
      options?.shouldPoll?.(query.state.data) ? CHAT_POLL_MS : false,
  });
}

export function useUpsertDeploymentChatConversation(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      conversationId,
      title,
    }: {
      conversationId: string;
      title: string;
    }) =>
      api.upsertDeploymentChatConversation(deploymentId, conversationId, {
        title,
      }),
    onSuccess: (_data, { conversationId }) => {
      void queryClient.invalidateQueries({
        queryKey: chatKeys.conversations(deploymentId),
      });
      void queryClient.invalidateQueries({
        queryKey: chatKeys.conversation(deploymentId, conversationId),
      });
    },
  });
}

export function useDeleteDeploymentChatConversation(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (conversationId: string) =>
      api.deleteDeploymentChatConversation(deploymentId, conversationId),
    onSuccess: (_data, conversationId) => {
      queryClient.removeQueries({
        queryKey: chatKeys.conversation(deploymentId, conversationId),
      });
      void queryClient.invalidateQueries({
        queryKey: chatKeys.conversations(deploymentId),
      });
    },
  });
}
