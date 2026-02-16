import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { deploymentKeys } from './keys';

export function useDeployments(account: string, enabled = true) {
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
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.undeployAgent.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

export function useActiveDeploymentSpec(account: string, name: string, enabled = true) {
  return useQuery({
    queryKey: deploymentKeys.spec(account, name),
    queryFn: () => api.getActiveDeploymentSpec(account, name),
    enabled: !!account && !!name && enabled,
  });
}

export function useDeploymentHistory(account: string, name: string, enabled = true) {
  return useQuery({
    queryKey: deploymentKeys.history(account, name),
    queryFn: () => api.getDeploymentHistory(account, name),
    enabled: !!account && !!name && enabled,
  });
}

export function useConfigMapData(account: string, namespace: string, cmname: string, enabled = true) {
  return useQuery({
    queryKey: deploymentKeys.configmap(account, namespace, cmname),
    queryFn: () => api.getConfigMapData(namespace, cmname, account),
    enabled: !!account && !!namespace && !!cmname && enabled,
  });
}

export function useSecretKeys(account: string, namespace: string, secretName: string, enabled = true) {
  return useQuery({
    queryKey: deploymentKeys.secretKeys(account, namespace, secretName),
    queryFn: () => api.getSecretKeys(namespace, secretName, account),
    enabled: !!account && !!namespace && !!secretName && enabled,
  });
}

export function useRestartPod(account: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.restartPod.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}
