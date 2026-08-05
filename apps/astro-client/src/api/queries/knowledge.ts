import { keepPreviousData, useInfiniteQuery, useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { knowledgeKeys } from "./keys";
import type {
  ConnectKnowledgeStoreInput,
  UpdateKnowledgeCredentialsInput,
  KnowledgeStore,
} from "@/lib/api";
import type { UserResourceScopeSelection } from "@/lib/user-resource-scope";
import type { UserResourceListParams } from "@/lib/user-resource-list-params";
import {
  isUserResourceQueryEnabled,
  nextUserResourceCursor,
  USER_RESOURCE_PAGE_SIZE,
  USER_RESOURCE_STALE_TIME_MS,
} from "./user-resources";

export const USER_KNOWLEDGE_PAGE_SIZE = USER_RESOURCE_PAGE_SIZE;

function invalidateKnowledgeLists(
  queryClient: ReturnType<typeof useQueryClient>,
  account: string,
) {
  queryClient.invalidateQueries({ queryKey: knowledgeKeys.all(account) });
  queryClient.invalidateQueries({ queryKey: knowledgeKeys.visibleLists });
}

export function useUserKnowledgeStores(
  scope: UserResourceScopeSelection,
  params: UserResourceListParams = {},
  enabled = true,
) {
  const api = useApiClient();
  const listParams = { ...params, limit: USER_KNOWLEDGE_PAGE_SIZE };
  return useInfiniteQuery({
    queryKey: knowledgeKeys.visibleList(scope, listParams),
    queryFn: ({ pageParam }) => api.listUserKnowledgeStores(scope, {
      ...listParams,
      cursor: pageParam,
    }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: nextUserResourceCursor,
    enabled: isUserResourceQueryEnabled(scope, enabled),
    staleTime: USER_RESOURCE_STALE_TIME_MS,
    refetchInterval: (query) => {
      const pages = query.state.data?.pages ?? [];
      const transitional = pages.some((page) =>
        page.stores.some((store) =>
          ["connecting", "pending-acceptance"].includes(store.status),
        ),
      );
      return transitional ? 3000 : false;
    },
  });
}

export function useKnowledgeStores(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: knowledgeKeys.all(account),
    queryFn: () => api.listKnowledgeStores(account),
    enabled: !!account && enabled,
    placeholderData: keepPreviousData,
    refetchInterval: (query) => {
      const stores = query.state.data ?? [];
      const hasTransitional = stores.some((s) =>
        ["connecting", "pending-acceptance"].includes(s.status)
      );
      return hasTransitional ? 3000 : false;
    },
  });
}

export function useKnowledgeStore(account: string, name: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: knowledgeKeys.detail(account, name),
    queryFn: () => api.getKnowledgeStore(account, name),
    enabled: !!account && !!name && enabled,
    placeholderData: keepPreviousData,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status && ["connecting", "pending-acceptance"].includes(status)) {
        return 3000;
      }
      return false;
    },
  });
}

export function useKnowledgeCredentials(account: string, name: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: knowledgeKeys.credentials(account, name),
    queryFn: () => api.getKnowledgeCredentials(account, name),
    enabled: !!account && !!name && enabled,
    retry: false,
  });
}

export function useConnectKnowledgeStore(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<KnowledgeStore, Error, ConnectKnowledgeStoreInput>({
    mutationFn: (data) => api.connectKnowledgeStore(account, data),
    onSuccess: () => {
      invalidateKnowledgeLists(queryClient, account);
    },
  });
}

export function useDeleteKnowledgeStore(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<{ message: string }, Error, { name: string }>({
    mutationFn: ({ name }) => api.deleteKnowledgeStore(account, name),
    onSuccess: () => {
      invalidateKnowledgeLists(queryClient, account);
    },
  });
}

// useUpdateKnowledgeCredentials updates a connected store's connection details.
// On success the store's status/error and credentials may change, so refresh the
// list, the store detail, and the credentials query.
export function useUpdateKnowledgeCredentials(account: string, name: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<KnowledgeStore, Error, UpdateKnowledgeCredentialsInput>({
    mutationFn: (data) => api.updateKnowledgeCredentials(account, name, data),
    onSuccess: () => {
      invalidateKnowledgeLists(queryClient, account);
      queryClient.invalidateQueries({ queryKey: knowledgeKeys.detail(account, name) });
      queryClient.invalidateQueries({ queryKey: knowledgeKeys.credentials(account, name) });
    },
  });
}

// useRecheckKnowledgeStore re-resolves a connected store's endpoint and fixes
// its host. Refreshes both the list and the affected store's detail.
export function useRecheckKnowledgeStore(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<KnowledgeStore, Error, { name: string }>({
    mutationFn: ({ name }) => api.recheckKnowledgeStore(account, name),
    onSuccess: (store) => {
      invalidateKnowledgeLists(queryClient, account);
      queryClient.invalidateQueries({ queryKey: knowledgeKeys.detail(account, store.name) });
    },
  });
}
