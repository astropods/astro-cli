import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { adminKeys } from "./keys";
import type {
  ListDeploymentsResponse,
  GetDeploymentResponse,
  ListAccountsResponse,
  ListAgentsResponse,
  GetAgentBuildsResponse,
  ClusterStatusResponse,
  ListImagesResponse,
  GetSchemaResponse,
  QueryDatabaseResponse,
  GetPodLogsResponse,
  GetPodEnvResponse,
} from "@/types/admin";

export function useDeployments() {
  return useQuery({
    queryKey: adminKeys.deployments(),
    queryFn: () => api.get<ListDeploymentsResponse>("/api/admin/deployments"),
  });
}

export function useDeployment(namespace: string) {
  return useQuery({
    queryKey: adminKeys.deployment(namespace),
    queryFn: () =>
      api.get<GetDeploymentResponse>(
        `/api/admin/deployments/${encodeURIComponent(namespace)}`
      ),
    enabled: !!namespace,
  });
}

export function useDeleteDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (namespace: string) =>
      api.del(`/api/admin/deployments/${encodeURIComponent(namespace)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useRestartDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (namespace: string) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(namespace)}/restart`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useAccounts() {
  return useQuery({
    queryKey: adminKeys.accounts(),
    queryFn: () => api.get<ListAccountsResponse>("/api/admin/accounts"),
  });
}

export function useRenameAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, newName }: { id: string; newName: string }) =>
      api.put(`/api/admin/accounts/${encodeURIComponent(id)}/rename`, {
        new_name: newName,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.accounts() });
    },
  });
}

export function useAgents() {
  return useQuery({
    queryKey: adminKeys.agents(),
    queryFn: () => api.get<ListAgentsResponse>("/api/admin/agents"),
  });
}

export function useAgentBuilds(account: string, name: string) {
  return useQuery({
    queryKey: adminKeys.agentBuilds(account, name),
    queryFn: () =>
      api.get<GetAgentBuildsResponse>(
        `/api/admin/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/builds`
      ),
    enabled: !!account && !!name,
  });
}

export function useClusterStatus(namespace: string) {
  return useQuery({
    queryKey: adminKeys.clusterStatus(namespace),
    queryFn: () =>
      api.get<ClusterStatusResponse>(
        `/api/admin/cluster-status?namespace=${encodeURIComponent(namespace)}`
      ),
    enabled: !!namespace,
  });
}

export function useImages() {
  return useQuery({
    queryKey: adminKeys.images(),
    queryFn: () => api.get<ListImagesResponse>("/api/admin/images"),
  });
}

export function useSchema() {
  return useQuery({
    queryKey: adminKeys.schema(),
    queryFn: () => api.get<GetSchemaResponse>("/api/admin/schema"),
  });
}

export function useQueryDatabase() {
  return useMutation({
    mutationFn: (query: string) =>
      api.post<QueryDatabaseResponse>("/api/admin/query", { query }),
  });
}

export function usePodLogs(namespace: string, pod: string) {
  return useQuery({
    queryKey: adminKeys.podLogs(namespace, pod),
    queryFn: () =>
      api.get<GetPodLogsResponse>(
        `/api/admin/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/logs`
      ),
    enabled: !!namespace && !!pod,
  });
}

export function usePodEnv(namespace: string, pod: string) {
  return useQuery({
    queryKey: adminKeys.podEnv(namespace, pod),
    queryFn: () =>
      api.get<GetPodEnvResponse>(
        `/api/admin/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/env`
      ),
    enabled: !!namespace && !!pod,
  });
}
