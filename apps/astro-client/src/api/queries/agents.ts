import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type AgentsListResponse, type Agent, type DeploymentTemplate, type DeployResponse, type DeploymentsListResponse } from '../../lib/api';
import { agentKeys, deploymentKeys } from './keys';

interface AccountAgentsQueryOptions {
  refetchInterval?: number | false;
  refetchIntervalInBackground?: boolean;
}

interface AgentQueryOptions {
  initialData?: Agent;
  enabled?: boolean;
  refetchInterval?: number | false;
  refetchIntervalInBackground?: boolean;
}

export function useAgents(opts?: { initialData?: AgentsListResponse }) {
  return useQuery({
    queryKey: agentKeys.all,
    queryFn: () => api.listAgents(),
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useAccountAgents(account: string, enabled = true, opts?: AccountAgentsQueryOptions) {
  return useQuery({
    queryKey: agentKeys.byAccount(account),
    queryFn: () => api.listAccountAgents(account),
    enabled: !!account && enabled,
    refetchInterval: opts?.refetchInterval,
    refetchIntervalInBackground: opts?.refetchIntervalInBackground ?? false,
  });
}

export function useAgent(account: string, name: string, opts?: AgentQueryOptions) {
  return useQuery({
    queryKey: agentKeys.detail(account, name),
    queryFn: () => api.getAgent(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
    refetchInterval: opts?.refetchInterval,
    refetchIntervalInBackground: opts?.refetchIntervalInBackground ?? false,
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
    onSuccess: (data: DeployResponse) => {
      // Optimistically patch build_id so the "new build" badge clears before the server catches up
      queryClient.setQueryData<DeploymentsListResponse>(
        deploymentKeys.all(account),
        (prev) => {
          if (!prev) return prev;
          return {
            ...prev,
            deployments: prev.deployments.map((d) =>
              d.name === data.name ? { ...d, build_id: data.build_id } : d,
            ),
          };
        },
      );

      queryClient.invalidateQueries({ queryKey: agentKeys.template(account, agentName) });
      queryClient.invalidateQueries({ queryKey: agentKeys.detail(account, agentName) });
      queryClient.invalidateQueries({ queryKey: agentKeys.byAccount(account) });
    },
  });
}

export function useValidateDeployment() {
  return useMutation({
    mutationFn: api.validateDeployment.bind(api),
  });
}

