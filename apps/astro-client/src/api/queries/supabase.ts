import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { supabaseKeys } from "./keys";

export function useSupabaseStatus(account: string, opts?: { enabled?: boolean }) {
  const api = useApiClient();
  return useQuery({
    queryKey: supabaseKeys.status(account),
    queryFn: () => api.supabaseStatus(account),
    enabled: opts?.enabled !== false && !!account,
  });
}

export function useSupabaseConnect(account: string) {
  const api = useApiClient();
  return useMutation({
    mutationFn: (redirectTo: string) => api.supabaseConnect(account, redirectTo),
  });
}

export function useSupabaseProjects(account: string, opts?: { enabled?: boolean }) {
  const api = useApiClient();
  return useQuery({
    queryKey: supabaseKeys.projects(account),
    queryFn: () => api.supabaseListProjects(account),
    enabled: opts?.enabled !== false && !!account,
    retry: false,
  });
}

export function useSupabaseProjectHealth(account: string, ref: string, opts?: { enabled?: boolean }) {
  const api = useApiClient();
  return useQuery({
    queryKey: supabaseKeys.projectHealth(account, ref),
    queryFn: () => api.supabaseProjectHealth(account, ref),
    enabled: opts?.enabled !== false && !!account && !!ref,
    retry: false,
  });
}

export function useSupabaseDisconnect(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.supabaseDisconnect(account),
    onSuccess: () => {
      queryClient.setQueryData(supabaseKeys.status(account), { connected: false });
      queryClient.removeQueries({ queryKey: supabaseKeys.projects(account) });
    },
  });
}
