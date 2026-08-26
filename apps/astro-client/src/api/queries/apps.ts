import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { appKeys } from "./keys";
import type { AppListResponse, AppScopesResponse, CreateAppResponse, NewAppSecret } from "@/lib/api";

export function useApps(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery<AppListResponse>({
    queryKey: appKeys.all(account),
    queryFn: () => api.listApps(account),
    enabled: enabled && !!account,
  });
}

export function useAppScopes(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery<AppScopesResponse>({
    queryKey: appKeys.scopes(account),
    queryFn: () => api.listAppScopes(account),
    enabled: enabled && !!account,
    staleTime: 5 * 60_000,
  });
}

export function useCreateApp(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<CreateAppResponse, Error, { name: string; description?: string; scopes?: string[] }>({
    mutationFn: (body) => api.createApp(account, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: appKeys.all(account) });
    },
  });
}

export function useDeleteApp(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (id) => api.deleteApp(account, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: appKeys.all(account) });
    },
  });
}

export function useCreateAppSecret(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<NewAppSecret, Error, string>({
    mutationFn: (id) => api.createAppSecret(account, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: appKeys.all(account) });
    },
  });
}

export function useDeleteAppSecret(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation<void, Error, { id: string; secretId: string }>({
    mutationFn: ({ id, secretId }) => api.deleteAppSecret(account, id, secretId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: appKeys.all(account) });
    },
  });
}
