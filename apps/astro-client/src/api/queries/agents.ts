import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type AgentsListResponse, type Agent, type DeploymentTemplate, type DeployResponse, type DeploymentsListResponse } from '../../lib/api';
import { agentKeys, deploymentKeys } from './keys';

interface AgentQueryOptions {
  initialData?: Agent;
  enabled?: boolean;
}

export function useAgents(opts?: { initialData?: AgentsListResponse }) {
  return useQuery({
    queryKey: agentKeys.all,
    queryFn: () => api.listAgents(),
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useAccountAgents(account: string, opts?: { enabled?: boolean; initialData?: AgentsListResponse }) {
  const enabled = opts?.enabled ?? true;
  return useQuery({
    queryKey: agentKeys.byAccount(account),
    queryFn: () => api.listAccountAgents(account),
    enabled: !!account && enabled,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useAgent(account: string, name: string, opts?: AgentQueryOptions) {
  return useQuery({
    queryKey: agentKeys.detail(account, name),
    queryFn: () => api.getAgent(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
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
    onMutate: async () => {
      // Immediate UX feedback: flip existing deployment rows to deploying.
      queryClient.setQueryData<DeploymentsListResponse>(
        deploymentKeys.all(account),
        (old) => {
          if (!old) return old;
          const deployments = Array.isArray(old.deployments) ? old.deployments : [];
          return {
            ...old,
            deployments: deployments.map((d) =>
              d.name === agentName
                ? {
                    ...d,
                    status: "pending",
                    ready: 0,
                  }
                : d,
            ),
          };
        },
      );
    },
    onSuccess: (data: DeployResponse) => {
      // Optimistically patch build_id so the "new build" badge clears before the server catches up
      const prev = queryClient.getQueryData<DeploymentsListResponse>(deploymentKeys.all(account));
      const isExistingDeployment = prev?.deployments?.some((d) => d.name === data.name);

      if (isExistingDeployment) {
        queryClient.setQueryData<DeploymentsListResponse>(
          deploymentKeys.all(account),
          (old) => {
            if (!old) return old;
            const deployments = Array.isArray(old.deployments) ? old.deployments : [];
            return {
              ...old,
              deployments: deployments.map((d) =>
                d.name === data.name
                  ? {
                      ...d,
                      build_id: data.build_id,
                      status: "pending",
                      ready: 0,
                    }
                  : d,
              ),
            };
          },
        );
      }

      // Always refresh after optimistic patch so server truth wins quickly.
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });

      queryClient.invalidateQueries({ queryKey: agentKeys.template(account, agentName) });
      queryClient.invalidateQueries({ queryKey: agentKeys.detail(account, agentName) });
      queryClient.invalidateQueries({ queryKey: agentKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: deploymentKeys.history(account, agentName) });
    },
  });
}

export function useValidateDeployment() {
  return useMutation({
    mutationFn: api.validateDeployment.bind(api),
  });
}

