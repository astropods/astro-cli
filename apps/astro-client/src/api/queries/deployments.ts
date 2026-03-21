import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useApiClient } from '../../lib/api-context';
import type { DeploymentsListResponse, UndeployResponse } from '@/lib/api';
import { deploymentKeys } from './keys';

export function useDeployments(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.all(account),
    queryFn: () => api.listDeployments(account),
    enabled: !!account && enabled,
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
  pod: string,
  container: string,
  timeRange = '1h',
  options?: { enabled?: boolean; refetchInterval?: number | false },
) {
  const api = useApiClient();
  const baseEnabled = !!deploymentId && !!pod && !!container;
  const enabled = (options?.enabled ?? true) && baseEnabled;
  return useQuery({
    queryKey: deploymentKeys.logs(deploymentId, pod, container, timeRange),
    queryFn: () => {
      const ms = TIME_RANGE_MS[timeRange];
      const since = ms ? new Date(Date.now() - ms).toISOString() : undefined;
      return api.getDeploymentLogs(deploymentId, pod, container, since);
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
    onSuccess: (_data, variables) => {
      // Optimistically remove from cache so it doesn't flash back to "deploying"
      // while K8s is still tearing down
      queryClient.setQueriesData(
        { queryKey: deploymentKeys.all(account) },
        (old: DeploymentsListResponse | undefined) => {
          if (!old) return old;
          const filtered = old.deployments.filter((d) => d.id !== variables.deployment_id);
          return { ...old, deployments: filtered, count: filtered.length };
        },
      );
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

export function usePauseDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.pauseDeployment.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

export function useWakeUpDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
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

export function useRestartPod(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.restartPod.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}
