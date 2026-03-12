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

export function useAccountAgents(account: string, enabled = true) {
  return useQuery({
    queryKey: agentKeys.byAccount(account),
    queryFn: () => api.listAccountAgents(account),
    enabled: !!account && enabled,
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

export function useDeploymentTemplate(account: string, name: string, opts?: { initialData?: DeploymentTemplate; enabled?: boolean }) {
  return useQuery({
    queryKey: agentKeys.template(account, name),
    queryFn: () => api.getDeploymentTemplate(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function usePrefilledDeploymentTemplate(account: string, name: string, deploymentId: string, opts?: { enabled?: boolean }) {
  return useQuery({
    queryKey: agentKeys.prefilledTemplate(account, name, deploymentId),
    queryFn: () => api.getPrefilledDeploymentTemplate(account, name, deploymentId),
    enabled: (opts?.enabled ?? true) && !!account && !!name && !!deploymentId,
  });
}

export function useDeployAgent(account: string, agentName: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.deployAgent.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      // Invalidate this agent's template cache (includes pre-filled templates)
      // so the settings page fetches fresh data after a deploy or redeploy.
      queryClient.invalidateQueries({ queryKey: agentKeys.template(account, agentName) });
    },
  });
}

export function useValidateDeployment() {
  return useMutation({
    mutationFn: api.validateDeployment.bind(api),
  });
}

