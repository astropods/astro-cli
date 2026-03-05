import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type AgentsListResponse, type Agent, type DeploymentTemplate } from '../../lib/api';
import { agentKeys, deploymentKeys } from './keys';

export function useAgents(opts?: { initialData?: AgentsListResponse }) {
  return useQuery({
    queryKey: agentKeys.all,
    queryFn: () => api.listAgents(),
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useAgent(account: string, name: string, opts?: { initialData?: Agent }) {
  return useQuery({
    queryKey: agentKeys.detail(account, name),
    queryFn: () => api.getAgent(account, name),
    enabled: !!account && !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useDeploymentTemplate(account: string, name: string, opts?: { initialData?: DeploymentTemplate }) {
  return useQuery({
    queryKey: agentKeys.template(account, name),
    queryFn: () => api.getDeploymentTemplate(account, name),
    enabled: !!account && !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
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

