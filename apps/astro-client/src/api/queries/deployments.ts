import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useApiClient } from '../../lib/api-context';
import { deploymentKeys } from './keys';

export function useDeployments(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.all(account),
    queryFn: () => api.listDeployments(account),
    enabled: !!account && enabled,
  });
}

export function useDeploymentLogs(
  account: string,
  namespace: string,
  pod: string,
  container: string,
  tailLines?: number,
) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.logs(account, namespace, pod, container, tailLines),
    queryFn: () => api.getDeploymentLogs(namespace, pod, container, account, tailLines),
    enabled: !!account && !!namespace && !!pod && !!container,
    // Logs are ephemeral — always refetch, never serve stale
    staleTime: 0,
    gcTime: 1000 * 30,
  });
}

export function useUndeployAgent(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.undeployAgent.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
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

export function useDeploymentHistory(account: string, name: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.history(account, name),
    queryFn: () => api.getDeploymentHistory(account, name),
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
