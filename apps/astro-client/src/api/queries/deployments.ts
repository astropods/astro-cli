import { useEffect, useMemo, useRef } from 'react';
import { keepPreviousData, useQuery, useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useApiClient } from '../../lib/api-context';
import type { LogEntry } from '@/lib/log-utils';
import type {
  AgentDeployment,
  DeploymentsListResponse,
  PodMetricsRange,
  UndeployResponse,
} from '@/lib/api';
import { deploymentKeys } from './keys';

// Powers the cross-account quick switcher on the agent detail page.
// Refreshed by mutation invalidation, not polling — a mutation in another
// tab won't surface here until this tab next mounts the query.
export function useDeploymentsSummary() {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.summary,
    queryFn: () => api.getDeploymentsSummary(),
    staleTime: 60_000,
  });
}

export function useDeployments(account: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.all(account),
    queryFn: () => api.listDeployments(account),
    enabled: !!account && enabled,
    // Keep showing the previous account's deployments while the new account's
    // list fetches. Prevents the dashboard from flashing empty on org switch.
    placeholderData: keepPreviousData,
    refetchInterval: (query) => {
      const deployments = query.state.data?.deployments ?? [];
      const hasTransitional = deployments.some((deployment) => {
        const status = deployment.status?.toLowerCase?.() ?? "";
        return (
          status === "pending" ||
          status === "provisioning" ||
          status === "deploying" ||
          status === "undeploying"
        );
      });
      return hasTransitional ? 3000 : false;
    },
  });
}

export function useDeployment(
  id: string,
  enabled = true,
  options?: { initialData?: { deployment: AgentDeployment } },
) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.detail(id),
    queryFn: () => api.getDeployment(id),
    enabled: !!id && enabled,
    initialData: options?.initialData,
    initialDataUpdatedAt: options?.initialData ? 0 : undefined,
    // No polling. The record is DB-only and its volatile fields (status
    // enum, ingress URLs) are refreshed via the status-transition effect in
    // useDeploymentRuntime (invalidates the detail prefix on flip-to-active)
    // and via invalidateDeployment on mutations.
  });
}

// useDeploymentStatus fetches the coarse, server-derived deployment status
// the UI renders (toggle label, history badge, deployment tile). The server
// joins DB status + K8s readiness in one place — the client doesn't have to
// reconcile two queries across timing windows, and the status badge never
// gets stuck in a transitional state because the runtime cache is stale.
// Polls every 3s while transitional (deploying/undeploying), idles otherwise.
export function useDeploymentStatus(id: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.status(id),
    queryFn: () => api.getDeploymentStatus(id),
    enabled: !!id && enabled,
    refetchInterval: (query) => {
      const s = query.state.data?.value;
      return s === "deploying" || s === "undeploying" ? 3000 : false;
    },
  });
}

// useDeploymentRuntime fetches the workload-level K8s view (pod containers,
// restart counts, runs). Only the AgentDeployments page needs this — the
// status badge / toggle should consume useDeploymentStatus instead. Polls
// while the deployment is transitional so the pod grid catches up after
// pause/resume/deploy.
//
// On the deploying → active transition we invalidate the entire deployment
// detail subtree once. The 3s poll loop halts as soon as status becomes
// active, but the LAST polled snapshot was taken while pods were still
// cycling — without this refetch the pod grid would render stale (e.g. show
// only the old replica until the next focus/remount). Invalidating the
// detail key prefix also refreshes the record (so URLs surfaced late in the
// deploy show up immediately) since runtime + status keys are children of
// detail.
export function useDeploymentRuntime(id: string, enabled = true) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  const statusQuery = useDeploymentStatus(id, enabled);
  const status = statusQuery.data?.value;
  const prevStatusRef = useRef<typeof status>(undefined);
  useEffect(() => {
    if (prevStatusRef.current && prevStatusRef.current !== "active" && status === "active") {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(id) });
    }
    prevStatusRef.current = status;
  }, [status, id, queryClient]);

  return useQuery({
    queryKey: deploymentKeys.runtime(id),
    queryFn: () => api.getDeploymentRuntime(id),
    enabled: !!id && enabled,
    refetchInterval: (query) => {
      if (status === "deploying" || status === "undeploying") return 3000;
      if (status === "active") {
        // Running but the workload list hasn't shown up yet — fast poll
        // until pods surface.
        if (!(query.state.data?.runtime?.workloads?.length ?? 0)) return 3000;
        // Steady-state active: slow background poll so live signals (pod
        // restarts, container crashes, age) refresh without a manual nudge.
        return 60_000;
      }
      return false;
    },
  });
}


const TIME_RANGE_MS: Record<string, number> = {
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '6h': 6 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 60 * 1000,
};

const LOG_PAGE_SIZE = 500;

export function useDeploymentLogs(
  deploymentId: string,
  workloadName: string,
  container: string,
  timeRange = '1h',
  timezone = 'UTC',
  options?: { enabled?: boolean },
) {
  const api = useApiClient();
  const baseEnabled = !!deploymentId && !!workloadName && !!container;
  const enabled = (options?.enabled ?? true) && baseEnabled;

  const query = useInfiniteQuery({
    queryKey: deploymentKeys.logs(deploymentId, workloadName, container, timeRange, timezone),
    queryFn: ({ pageParam }: { pageParam: string | undefined }) => {
      const ms = TIME_RANGE_MS[timeRange];
      const since = ms ? new Date(Date.now() - ms).toISOString() : undefined;
      return api.getDeploymentLogs(deploymentId, workloadName, container, since, timezone, {
        tailLines: LOG_PAGE_SIZE,
        direction: 'backward',
        until: pageParam,
      });
    },
    // Cursor for the next (older) page is the oldest timestamp in the last fetched page.
    getNextPageParam: (lastPage: LogEntry[]) =>
      lastPage.length >= LOG_PAGE_SIZE ? (lastPage[0]?.timestamp ?? undefined) : undefined,
    initialPageParam: undefined as string | undefined,
    enabled,
    staleTime: 0,
    gcTime: 1000 * 30,
  });

  // Pages are ordered newest-first; reverse so the flat array is oldest→newest.
  const logs = useMemo(
    () => [...(query.data?.pages ?? [])].reverse().flat(),
    [query.data?.pages],
  );

  return {
    data: logs,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    refetch: query.refetch,
    loadMore: () => query.fetchNextPage(),
    isLoadingMore: query.isFetchingNextPage,
    hasMore: query.hasNextPage,
  };
}

export function useLastErrorLog(
  deploymentId: string,
  workloadName: string,
  container: string,
  enabled = true,
) {
  const api = useApiClient();
  const baseEnabled = !!deploymentId && !!workloadName && !!container && enabled;
  return useQuery({
    queryKey: deploymentKeys.lastError(deploymentId, workloadName, container),
    queryFn: () =>
      api.getDeploymentLogs(deploymentId, workloadName, container, undefined, undefined, {
        level: 'error',
        direction: 'backward',
        tailLines: 1,
      }),
    enabled: baseEnabled,
    staleTime: 30_000,
    gcTime: 5 * 60 * 1000,
    refetchInterval: false,
    retry: 1,
  });
}

export function useDeploymentEvents(deploymentId: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.events(deploymentId),
    queryFn: () => api.getDeploymentEvents(deploymentId),
    enabled: !!deploymentId && enabled,
    refetchInterval: 10_000,
    staleTime: 0,
  });
}

/**
 * CPU and memory time series for one pod. The server queries Prometheus, so
 * results lag scrape interval (~30s). Refresh cadence scales with the window
 * — short ranges get fresh data more often than week-long charts.
 */
export function usePodMetrics(
  deploymentId: string,
  pod: string,
  range: PodMetricsRange,
  enabled = true,
) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.podMetrics(deploymentId, pod, range),
    queryFn: () => api.getPodMetrics(deploymentId, pod, range),
    enabled: !!deploymentId && !!pod && enabled,
    refetchInterval: range === '1h' ? 30_000 : range === '6h' ? 60_000 : 5 * 60_000,
    staleTime: 15_000,
    placeholderData: keepPreviousData,
  });
}

export function useUndeployAgent(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<UndeployResponse, Error, { deployment_id: string }>({
    mutationFn: api.undeployAgent.bind(api),
    onSuccess: (data, variables) => {
      // Keep the deployment visible and mark it undeploying so users can track teardown.
      queryClient.setQueriesData(
        { queryKey: deploymentKeys.all(account) },
        (old: DeploymentsListResponse | undefined) => {
          if (!old) return old;
          const updated = old.deployments.map((d) =>
            d.id === variables.deployment_id
              ? {
                  ...d,
                  status: data?.status || "undeploying",
                }
              : d,
          );
          return { ...old, deployments: updated };
        },
      );
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      // The summary key isn't a prefix of all(account), so it's not swept by
      // the line above. Stale it explicitly so the agent-detail quick
      // switcher refetches on its next mount.
      queryClient.invalidateQueries({ queryKey: deploymentKeys.summary });
    },
  });
}

// invalidateDeployment refetches every deployment-scoped query for the id:
// the record, the runtime, and the status. The single invalidate call is
// enough because runtime() and status() keys are children of detail() —
// TanStack matches by prefix by default, so invalidating the detail key
// invalidates the whole subtree. Mutations that change pod state (pause /
// resume / restart) must hit all three: invalidating only the record would
// leave the runtime cached at its pre-mutation snapshot, which manifests as
// the toggle/history badge sticking on "Resuming"/"Deploying" after the
// mutation completes.
function invalidateDeployment(queryClient: ReturnType<typeof useQueryClient>, deploymentId: string) {
  queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(deploymentId) });
}

export function useRestartDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<{ status: string; pods: string[] }, Error, { deploymentId: string }>({
    mutationFn: api.restartDeployment.bind(api),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      invalidateDeployment(queryClient, variables.deploymentId);
    },
  });
}

export function useRestartPod() {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<{ status: string; pod: string }, Error, { deploymentId: string; podName: string }>({
    mutationFn: api.restartPod.bind(api),
    onSuccess: (_, variables) => {
      invalidateDeployment(queryClient, variables.deploymentId);
    },
  });
}

export function useStopDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<{ status: string; deployment_id: string }, Error, { deploymentId: string }>({
    mutationFn: api.stopDeployment.bind(api),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      invalidateDeployment(queryClient, variables.deploymentId);
    },
  });
}

export function useWakeUpDeployment(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<unknown, Error, { deploymentId: string }>({
    mutationFn: api.wakeupDeployment.bind(api),
    onSuccess: (_data, variables) => {
      queryClient.setQueriesData(
        { queryKey: deploymentKeys.all(account) },
        (old: DeploymentsListResponse | undefined) => {
          if (!old) return old;
          return {
            ...old,
            deployments: old.deployments.map((d) =>
              d.id === variables.deploymentId ? { ...d, status: 'pending' } : d
            ),
          };
        },
      );
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
      invalidateDeployment(queryClient, variables.deploymentId);
    },
  });
}

export function useActiveDeploymentSpec(account: string, name: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.spec(account, name),
    queryFn: () => api.getActiveDeploymentSpec(account, name),
    enabled: !!account && !!name && enabled,
    placeholderData: keepPreviousData,
  });
}

export function useDeploymentHistory(
  account: string,
  name: string,
  deploymentId?: string,
  enabled = true,
  options?: { refetchInterval?: number | false },
) {
  const api = useApiClient();
  return useQuery({
    queryKey: deploymentKeys.history(account, name, deploymentId),
    queryFn: () => api.getDeploymentHistory(account, name, deploymentId),
    enabled: !!account && !!name && enabled,
    placeholderData: keepPreviousData,
    refetchInterval: options?.refetchInterval,
  });
}

export function useTriggerIngestion(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: api.triggerIngestion.bind(api),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

// Deployment avatar mutations
export function useUploadDeploymentAvatar(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, file }: { id: string; file: Blob }) =>
      api.uploadDeploymentAvatar(id, file),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}

export function useUpdateDeploymentDisplayName(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (displayName: string) =>
      api.updateDeploymentDisplayName(deploymentId, displayName),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(deploymentId) });
    },
  });
}

export function useDeleteDeploymentAvatar(account: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api.deleteDeploymentAvatar(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    },
  });
}
