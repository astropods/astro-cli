import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { knowledgeKeys } from "./keys";
import type {
  CreateKnowledgeStoreInput,
  ConnectKnowledgeStoreInput,
  KnowledgeStore,
  KnowledgeStoreListResponse,
} from "@/lib/api";

export function useKnowledgeStores(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: knowledgeKeys.all(account),
    queryFn: () => api.listKnowledgeStores(account),
    enabled: !!account && enabled,
    refetchInterval: (query) => {
      const stores = query.state.data?.stores ?? [];
      const hasTransitional = stores.some((s) =>
        ["provisioning", "connecting", "pending-acceptance"].includes(s.status)
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
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status && ["provisioning", "connecting", "pending-acceptance"].includes(status)) {
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

export function useCreateKnowledgeStore(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<KnowledgeStore, Error, CreateKnowledgeStoreInput>({
    mutationFn: (data) => api.createKnowledgeStore(account, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: knowledgeKeys.all(account) });
    },
  });
}

export function useConnectKnowledgeStore(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<KnowledgeStore, Error, ConnectKnowledgeStoreInput>({
    mutationFn: (data) => api.connectKnowledgeStore(account, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: knowledgeKeys.all(account) });
    },
  });
}

export function useDeleteKnowledgeStore(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<{ message: string }, Error, { name: string }>({
    mutationFn: ({ name }) => api.deleteKnowledgeStore(account, name),
    onSuccess: (_data, variables) => {
      queryClient.setQueriesData(
        { queryKey: knowledgeKeys.all(account) },
        (old: KnowledgeStoreListResponse | undefined) => {
          if (!old) return old;
          return {
            ...old,
            stores: old.stores.filter((s) => s.name !== variables.name),
          };
        },
      );
      queryClient.invalidateQueries({ queryKey: knowledgeKeys.all(account) });
    },
  });
}
