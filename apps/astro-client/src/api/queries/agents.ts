import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { agentKeys, deploymentKeys } from './keys';

export function useAgents() {
  return useQuery({
    queryKey: agentKeys.all,
    queryFn: () => api.listAgents(),
  });
}

export function useAgent(name: string) {
  return useQuery({
    queryKey: agentKeys.detail(name),
    queryFn: () => api.getAgent(name),
    enabled: !!name,
  });
}

export function useAgentVersion(name: string, version: string) {
  return useQuery({
    queryKey: agentKeys.version(name, version),
    queryFn: () => api.getAgentVersion(name, version),
    enabled: !!name && !!version,
  });
}

export function useAgentConfig(name: string, version: string) {
  return useQuery({
    queryKey: agentKeys.config(name, version),
    queryFn: () => api.getAgentConfig(name, version),
    enabled: !!name && !!version,
  });
}

export function useDeployAgent() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.deployAgent.bind(api),
    onSuccess: () => {
      // Deploying changes the user's deployment list, not the agent registry
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all });
    },
  });
}
