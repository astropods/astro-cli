import { useQuery, useSuspenseQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useApiClient } from '../../lib/api-context';
import type { AgentDeployment, DeploymentsListResponse, UndeployResponse } from '@/lib/api';
import { deploymentKeys } from './keys';

/**
 * Returns true when workload container readiness hasn't caught up with the
 * deployment's desired replica count. This happens briefly after pause/resume:
 *  - After pause: replicas=0 but containers may still report ready:true
 *  - After resume: replicas>0 but containers may still report ready:false
 * We keep polling in these windows so the service accordion updates without
 * requiring a hard refresh.
 */
function hasContainerMismatch(dep: AgentDeployment | null | undefined): boolean {
  if (!dep) return false;
  const workloads = dep.workloads ?? [];
  if (dep.replicas === 0) {
    return workloads.some((wl) => (wl.containers ?? []).some((c) => c.ready));
  }
  return workloads.some((wl) => (wl.containers ?? []).some((c) => !c.ready));
}

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

function deploymentNeedsPolling(dep: AgentDeployment | null | undefined): boolean {
  if (!dep) return false;
  const status = dep.status?.toLowerCase?.() ?? "";
  const isTransitional =
    status === "pending" ||
    status === "provisioning" ||
    status === "deploying" ||
    status === "undeploying";
  // A Running deployment with no workloads is a transient K8s state (pods cycling during
  // a rolling update). Keep polling until workloads appear so the playground URL surfaces.
  const missingWorkloads = status === "running" && !(dep.workloads?.length ?? 0);
  return isTransitional || hasContainerMismatch(dep) || missingWorkloads;
}

export function useDeployment(id: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.detail(id),
    queryFn: () => api.getDeployment(id),
    enabled: !!id && enabled,
    refetchInterval: (query) => deploymentNeedsPolling(query.state.data?.deployment) ? 3000 : false,
  });
}


export function useDeploymentSuspense(id: string) {
  const api = useApiClient();
  return useSuspenseQuery({
    queryKey: deploymentKeys.detail(id),
    queryFn: () => api.getDeployment(id),
    refetchInterval: (query) => deploymentNeedsPolling(query.state.data?.deployment) ? 3000 : false,
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
  timezone = 'UTC',
  options?: { enabled?: boolean; refetchInterval?: number | false },
) {
  const api = useApiClient();
  const baseEnabled = !!deploymentId && !!workloadName && !!container;
  const enabled = (options?.enabled ?? true) && baseEnabled;
  return useQuery({
    queryKey: deploymentKeys.logs(deploymentId, workloadName, container, timeRange, timezone),
    queryFn: () => {
      const ms = TIME_RANGE_MS[timeRange];
      const since = ms ? new Date(Date.now() - ms).toISOString() : undefined;
      return api.getDeploymentLogs(deploymentId, workloadName, container, since, timezone);
    },
    enabled,
    refetchInterval: options?.refetchInterval,
    staleTime: 0,
    gcTime: 1000 * 30,
  });
}

export function useDeploymentEvents(deploymentId: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.events(deploymentId),
    queryFn: () => api.getDeploymentEvents(deploymentId),
    enabled: !!deploymentId && enabled,
    refetchInterval: 10_000,
    staleTime: 0,
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

export function useRestartDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<{ status: string; pods: string[] }, Error, { deploymentId: string }>({
    mutationFn: api.restartDeployment.bind(api),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(variables.deploymentId) });
    },
  });
}

export function useRestartPod() {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<{ status: string; pod: string }, Error, { deploymentId: string; podName: string }>({
    mutationFn: api.restartPod.bind(api),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(variables.deploymentId) });
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

export function useDeploymentHistory(
  account: string,
  name: string,
  deploymentId?: string,
  enabled = true,
  options?: { refetchInterval?: number | false },
) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.history(account, name, deploymentId),
    queryFn: () => api.getDeploymentHistory(account, name, deploymentId),
    enabled: !!account && !!name && enabled,
    refetchInterval: options?.refetchInterval,
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

