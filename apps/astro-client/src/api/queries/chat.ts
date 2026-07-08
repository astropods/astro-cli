/** TanStack Query bindings for GET/PUT/POST /deployments/:id/chat (web /chat UI). */
import { useCallback, useMemo, type MutableRefObject } from "react";
import {
  useMutation,
  useQueries,
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
import type { ChatAgent } from "@/lib/chat/types";
import { isChatListEligible } from "@/lib/deployment-utils";
import { CHAT_POLL_MS } from "@/lib/messaging/transport";
import { useDeploymentsSummary } from "./deployments";
import { chatKeys, deploymentKeys } from "./keys";

/**
 * Cross-account chat agents: every chat-eligible agent the user can reach,
 * regardless of which org/account it is deployed to. The deployments summary is
 * already membership-scoped server-side, so we use it to enumerate the user's
 * accounts, then fan out per-account deployment reads and keep the ones with web
 * messaging. This is why the chat page needs no account switcher.
 */
export function useChatAgents(enabled = true): {
  entries: ChatAgent[];
  /** All deployments across accounts, eligible or not — tells "no agents" from "no chat-enabled agents". */
  totalDeployments: number;
  isLoading: boolean;
  /** True when any read failed. A failed read reports 0 deployments, which looks identical to "no agents". */
  isError: boolean;
  /** Refetch all deployment queries. */
  refetch: () => void;
} {
  const api = useApiClient();
  const summary = useDeploymentsSummary();

  const accountNames = useMemo(
    () => (summary.data?.accounts ?? []).map((a) => a.name),
    [summary.data?.accounts],
  );

  const deploymentQueries = useQueries({
    queries: accountNames.map((name) => ({
      queryKey: deploymentKeys.all(name),
      queryFn: () => api.listDeployments(name),
      enabled: enabled && !!name,
      staleTime: 30_000,
    })),
  });

  const entries = useMemo<ChatAgent[]>(() => {
    const result: ChatAgent[] = [];
    accountNames.forEach((name, index) => {
      const deployments = deploymentQueries[index]?.data?.deployments ?? [];
      for (const deployment of deployments) {
        if (!isChatListEligible(deployment)) continue;
        result.push({ deployment, account: name });
      }
    });
    return result;
    // deploymentQueries identity changes per render; the data it carries is the
    // real input, so recomputing the merge is intentional.
  }, [accountNames, deploymentQueries]);

  const totalDeployments = useMemo(
    () =>
      deploymentQueries.reduce(
        (sum, q) => sum + (q.data?.deployments?.length ?? 0),
        0,
      ),
    [deploymentQueries],
  );

  const isLoading =
    summary.isLoading || deploymentQueries.some((q) => q.isLoading);

  const isError =
    summary.isError || deploymentQueries.some((q) => q.isError);

  const refetch = useCallback(() => {
    void summary.refetch();
    for (const q of deploymentQueries) void q.refetch();
  }, [summary, deploymentQueries]);

  return { entries, totalDeployments, isLoading, isError, refetch };
}

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

/** Bound on the agent/config request: the messaging proxy has no upstream
 *  timeout, so an unresponsive sidecar would otherwise hang ~60s and 5xx. */
const AGENT_CONFIG_TIMEOUT_MS = 10_000;

/** Agent self-reported config (system prompt + tools) for the inspector.
 *  Only runs when `enabled` (the caller gates on a ready agent), never retries
 *  (a hung sidecar would just multiply 5xx), and fails fast via an abort
 *  timeout so the proxy route doesn't accumulate long-hanging errors. */
export function useDeploymentAgentConfig(
  deploymentId: string,
  enabled = true,
) {
  const api = useApiClient();
  return useQuery({
    queryKey: chatKeys.agentConfig(deploymentId),
    queryFn: ({ signal }) =>
      api.getDeploymentAgentConfig(
        deploymentId,
        AbortSignal.any([signal, AbortSignal.timeout(AGENT_CONFIG_TIMEOUT_MS)]),
      ),
    enabled: enabled && !!deploymentId,
    staleTime: 60_000,
    retry: false,
    refetchOnWindowFocus: false,
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

export function useSetDeploymentChatConversationTitle(deploymentId: string) {
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
      api.setDeploymentChatConversationTitle(deploymentId, conversationId, {
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
