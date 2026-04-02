import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type BlueprintsListResponse, type Blueprint, type DeploymentTemplate, type DeployResponse, type DeploymentsListResponse } from '../../lib/api';
import { useApiClient } from '../../lib/api-context';
import { blueprintKeys, deploymentKeys } from './keys';

interface BlueprintQueryOptions {
  initialData?: Blueprint;
  enabled?: boolean;
}

export function useBlueprints(opts?: { initialData?: BlueprintsListResponse }) {
  return useQuery({
    queryKey: blueprintKeys.all,
    queryFn: () => api.listBlueprints(),
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useAccountBlueprints(account: string, opts?: { enabled?: boolean; initialData?: BlueprintsListResponse }) {
  const enabled = opts?.enabled ?? true;
  return useQuery({
    queryKey: blueprintKeys.byAccount(account),
    queryFn: () => api.listAccountBlueprints(account),
    enabled: !!account && enabled,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useBlueprint(account: string, name: string, opts?: BlueprintQueryOptions) {
  return useQuery({
    queryKey: blueprintKeys.detail(account, name),
    queryFn: () => api.getBlueprint(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function useDeploymentTemplate(account: string, name: string, opts?: { initialData?: DeploymentTemplate; enabled?: boolean }) {
  return useQuery({
    queryKey: blueprintKeys.template(account, name),
    queryFn: () => api.getDeploymentTemplate(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

export function usePrefilledDeploymentTemplate(account: string, name: string, deploymentId: string, opts?: { enabled?: boolean; revision?: number }) {
  const revision = opts?.revision;
  return useQuery({
    queryKey: blueprintKeys.prefilledTemplate(account, name, deploymentId, revision),
    queryFn: () => api.getPrefilledDeploymentTemplate(account, name, deploymentId, revision),
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

      queryClient.invalidateQueries({ queryKey: blueprintKeys.template(account, agentName) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.detail(account, agentName) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: deploymentKeys.history(account, agentName) });
    },
  });
}

export function useValidateDeployment() {
  return useMutation({
    mutationFn: api.validateDeployment.bind(api),
  });
}

export function useArchiveBlueprint(account: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ name }: { name: string }) => api.archiveBlueprint(account, name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: blueprintKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.all });
    },
  });
}

// Blueprint avatar mutations
export function useUploadBlueprintAvatar() {
  const apiClient = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ account, name, file }: { account: string; name: string; file: Blob }) =>
      apiClient.uploadBlueprintAvatar(account, name, file),
    onSuccess: (data, { account, name }) => {
      // Immediately patch the cached blueprint with the new avatar URL so the UI
      // updates without waiting for the background refetch.
      queryClient.setQueryData<Blueprint>(blueprintKeys.detail(account, name), (old) => {
        if (!old) return old;
        return { ...old, avatar_url: data.avatar_url };
      });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.detail(account, name) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.all });
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}
