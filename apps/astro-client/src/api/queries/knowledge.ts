import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { knowledgeKeys } from "./keys";
import type {
  CreateKnowledgeStoreInput,
  ConnectKnowledgeStoreInput,
  KnowledgeStore,
} from "@/lib/api";

export function useKnowledgeStores(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: knowledgeKeys.all(account),
    queryFn: () => api.listKnowledgeStores(account),
    enabled: !!account && enabled,
    refetchInterval: (query) => {
      const stores = query.state.data ?? [];
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

const TIME_RANGE_MS: Record<string, number> = {
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
};

export function useKnowledgeLogs(
  account: string,
  name: string,
  timeRange = '1h',
  options?: { enabled?: boolean },
) {
  const api = useApiClient();
  const baseEnabled = !!account && !!name;
  const enabled = (options?.enabled ?? true) && baseEnabled;
  return useQuery({
    queryKey: knowledgeKeys.logs(account, name, timeRange),
    queryFn: () => {
      const ms = TIME_RANGE_MS[timeRange];
      const since = ms ? new Date(Date.now() - ms).toISOString() : undefined;
      return api.getKnowledgeLogs(account, name, since);
    },
    enabled,
    staleTime: 0,
    gcTime: 1000 * 30,
  });
}

export function useKnowledgeMetrics(account: string, name: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: knowledgeKeys.metrics(account, name),
    queryFn: () => api.getKnowledgeMetrics(account, name),
    enabled: !!account && !!name && enabled,
    refetchInterval: 30_000,
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: knowledgeKeys.all(account) });
    },
  });
}
