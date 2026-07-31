import { keepPreviousData, useQuery, useMutation, useQueryClient, type Query } from '@tanstack/react-query';
import { api, type BlueprintsListResponse, type Blueprint, type DeploymentSpec, type DeployResponse, type DeploymentsListResponse, type TemplateRequest } from '../../lib/api';
import { useApiClient } from '../../lib/api-context';
import { blueprintKeys, deploymentKeys, githubKeys } from './keys';

interface BlueprintQueryOptions {
  initialData?: Blueprint;
  enabled?: boolean;
  retry?: boolean | number;
  refetchInterval?: number | false | ((query: Query<Blueprint>) => number | false | undefined);
}

export function useBlueprints(opts?: {
  initialData?: BlueprintsListResponse;
}) {
  return useQuery({
    queryKey: blueprintKeys.all,
    queryFn: () => api.listBlueprints(),
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
  });
}

/** Profile, deployment history, etc. — unpaginated fetch (server max 100). Use useAccountBlueprintsList on /blueprints. */
export function useAccountBlueprints(
  account: string,
  opts?: {
    enabled?: boolean;
    initialData?: BlueprintsListResponse;
    refetchInterval?: number | false | ((query: Query<BlueprintsListResponse>) => number | false | undefined);
  },
) {
  return useQuery({
    queryKey: blueprintKeys.byAccount(account),
    queryFn: () => api.listAccountBlueprints(account),
    enabled: !!account && (opts?.enabled ?? true),
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
    placeholderData: keepPreviousData,
    refetchInterval: opts?.refetchInterval,
  });
}

export function useBlueprint(account: string, name: string, opts?: BlueprintQueryOptions) {
  return useQuery({
    queryKey: blueprintKeys.detail(account, name),
    queryFn: () => api.getBlueprint(account, name),
    enabled: (opts?.enabled ?? true) && !!account && !!name,
    initialData: opts?.initialData,
    initialDataUpdatedAt: opts?.initialData ? 0 : undefined,
    ...(opts?.retry !== undefined && { retry: opts.retry }),
    refetchInterval: opts?.refetchInterval,
  });
}

export function usePostDeploymentTemplate(account: string, name: string) {
  return useMutation({
    mutationFn: (body: TemplateRequest) => api.postDeploymentTemplate(account, name, body),
  });
}

export function useDeployAgent(account: string, agentName: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (args: { spec: DeploymentSpec; signature?: string }) =>
      api.deployAgent(args.spec, { signature: args.signature }),
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
                    }
                  : d,
              ),
            };
          },
        );
      }

      // Always refresh after optimistic patch so server truth wins quickly.
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      if (data.deployment_id) {
        queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(data.deployment_id) });
      }

      queryClient.invalidateQueries({ queryKey: blueprintKeys.detail(account, agentName) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: deploymentKeys.history(account, agentName) });
    },
  });
}

export function useValidateDeployment() {
  return useMutation({
    mutationFn: (args: { spec: DeploymentSpec; signature?: string }) =>
      api.validateDeployment(args.spec, { signature: args.signature }),
  });
}

export function useCreateBlueprint(account: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; visibility?: string }) => api.createBlueprint(account, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: blueprintKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.all });
    },
  });
}

export function useArchiveBlueprint(account: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ name }: { name: string }) => api.archiveBlueprint(account, name),
    onSuccess: (_, { name }) => {
      queryClient.invalidateQueries({ queryKey: blueprintKeys.byAccount(account) });
      queryClient.invalidateQueries({ queryKey: blueprintKeys.all });
      queryClient.invalidateQueries({ queryKey: githubKeys.accountConnections(account) });
      queryClient.invalidateQueries({ queryKey: githubKeys.status(account, name) });
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
