import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { deploymentKeys } from './keys';

export function useDeployments(enabled = true) {
  return useQuery({
    queryKey: deploymentKeys.all,
    queryFn: () => api.listDeployments(),
    enabled,
  });
}

export function useDeploymentLogs(
  name: string,
  version: string,
  pod: string,
  container: string,
  tailLines?: number,
) {
  return useQuery({
    queryKey: deploymentKeys.logs(name, version, pod, container, tailLines),
    queryFn: () => api.getDeploymentLogs(name, version, pod, container, tailLines),
    enabled: !!name && !!version && !!pod && !!container,
    // Logs are ephemeral — always refetch, never serve stale
    staleTime: 0,
    gcTime: 1000 * 30,
  });
}

export function useUndeployAgent() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.undeployAgent.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all });
    },
  });
}
