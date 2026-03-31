import { useQuery, useSuspenseQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useApiClient } from '../../lib/api-context';
import type { DeploymentsListResponse, UndeployResponse } from '@/lib/api';
import { deploymentKeys } from './keys';

export function useDeployments(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.all(account),
    queryFn: () => api.listDeployments(account),
    enabled: !!account && enabled,
    refetchInterval: (query) => {
      const deployments = query.state.data?.deployments ?? [];
      const hasTransitional = deployments.some((deployment) => {
        const status = deployment.status?.toLowerCase?.() ?? "";
        return (
          status === "pending" ||
          status === "provisioning" ||
          status === "deploying" ||
          status === "undeploying"
        );
      });
      return hasTransitional ? 3000 : false;
    },
  });
}

export function useDeployment(id: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.detail(id),
    queryFn: () => api.getDeployment(id),
    enabled: !!id && enabled,
    refetchInterval: (query) => {
      const status = query.state.data?.deployment?.status?.toLowerCase?.() ?? "";
      const isTransitional =
        status === "pending" ||
        status === "provisioning" ||
        status === "deploying" ||
        status === "undeploying";
      return isTransitional ? 3000 : false;
    },
  });
}

export function useDeploymentSuspense(id: string) {
  const api = useApiClient();
  return useSuspenseQuery({
    queryKey: deploymentKeys.detail(id),
    queryFn: () => api.getDeployment(id),
    refetchInterval: (query) => {
      const status = query.state.data?.deployment?.status?.toLowerCase?.() ?? "";
      const isTransitional =
        status === "pending" ||
        status === "provisioning" ||
        status === "deploying" ||
        status === "undeploying";
      return isTransitional ? 3000 : false;
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

export function useDeploymentLogs(
  deploymentId: string,
  workloadName: string,
  container: string,
  timeRange = '1h',
  options?: { enabled?: boolean; refetchInterval?: number | false },
) {
  const api = useApiClient();
  const baseEnabled = !!deploymentId && !!workloadName && !!container;
  const enabled = (options?.enabled ?? true) && baseEnabled;
  return useQuery({
    queryKey: deploymentKeys.logs(deploymentId, workloadName, container, timeRange),
    queryFn: () => {
      const ms = TIME_RANGE_MS[timeRange];
      const since = ms ? new Date(Date.now() - ms).toISOString() : undefined;
      return api.getDeploymentLogs(deploymentId, workloadName, container, since);
    },
    enabled,
    refetchInterval: options?.refetchInterval,
    staleTime: 0,
    gcTime: 1000 * 30,
  });
}

export function useUndeployAgent(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<UndeployResponse, Error, { deployment_id: string }>({
    mutationFn: api.undeployAgent.bind(api),
    onSuccess: (data, variables) => {
      // Keep the deployment visible and mark it undeploying so users can track teardown.
      queryClient.setQueriesData(
        { queryKey: deploymentKeys.all(account) },
        (old: DeploymentsListResponse | undefined) => {
          if (!old) return old;
          const updated = old.deployments.map((d) =>
            d.id === variables.deployment_id
              ? {
                  ...d,
                  status: data?.status || "undeploying",
                }
              : d,
          );
          return { ...old, deployments: updated };
        },
      );
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

export function useStopDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<{ status: string; deployment_id: string }, Error, { deploymentId: string }>({
    mutationFn: api.stopDeployment.bind(api),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(variables.deploymentId) });
    },
  });
}

export function useWakeUpDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<unknown, Error, { deploymentId: string }>({
    mutationFn: api.wakeupDeployment.bind(api),
    onSuccess: (_data, variables) => {
      queryClient.setQueriesData(
        { queryKey: deploymentKeys.all(account) },
        (old: DeploymentsListResponse | undefined) => {
          if (!old) return old;
          return {
            ...old,
            deployments: old.deployments.map((d) =>
              d.id === variables.deploymentId ? { ...d, status: 'pending', ready: 0 } : d
            ),
          };
        },
      );
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(variables.deploymentId) });
    },
  });
}

export function useActiveDeploymentSpec(account: string, name: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.spec(account, name),
    queryFn: () => api.getActiveDeploymentSpec(account, name),
    enabled: !!account && !!name && enabled,
  });
}

export function useDeploymentHistory(account: string, name: string, deploymentId?: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.history(account, name, deploymentId),
    queryFn: () => api.getDeploymentHistory(account, name, deploymentId),
    enabled: !!account && !!name && enabled,
  });
}

export function useTriggerIngestion(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.triggerIngestion.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

// Deployment avatar mutations
export function useUploadDeploymentAvatar(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, file }: { id: string; file: Blob }) =>
      api.uploadDeploymentAvatar(id, file),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

export function useDeleteDeploymentAvatar(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.deleteDeploymentAvatar(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

