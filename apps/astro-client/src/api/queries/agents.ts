import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { agentKeys, deploymentKeys, accountKeys } from './keys';

export function useAgents() {
  return useQuery({
    queryKey: agentKeys.all,
    queryFn: () => api.listAgents(),
  });
}

export function useAgent(account: string, name: string) {
  return useQuery({
    queryKey: agentKeys.detail(account, name),
    queryFn: () => api.getAgent(account, name),
    enabled: !!account && !!name,
  });
}

export function useDeploymentTemplate(account: string, name: string) {
  return useQuery({
    queryKey: agentKeys.template(account, name),
    queryFn: () => api.getDeploymentTemplate(account, name),
    enabled: !!account && !!name,
  });
}

export function useDeployAgent(account: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.deployAgent.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

export function useValidateDeployment() {
  return useMutation({
    mutationFn: api.validateDeployment.bind(api),
  });
}

export function usePublishAgent(account: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: { name: string; build_id: string; version: string }) =>
      api.publishAgent(account, data.name, { build_id: data.build_id, version: data.version }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentKeys.all });
      queryClient.invalidateQueries({ queryKey: accountKeys.profile });
    },
  });
}
