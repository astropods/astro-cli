import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import type {
  NetworkDirection,
  NetworkFlowsSort,
  NetworkGroupBy,
  NetworkMetric,
} from '@/lib/api';
import { networkKeys } from './keys';

// Network charts always re-fetch on mount but keep the prior payload visible
// during refetch so range/window changes don't flash.
const LIVE_QUERY_OPTS = {
  staleTime: 0,
  gcTime: 1000 * 60 * 30,
  placeholderData: keepPreviousData,
  retry: false,
  refetchOnWindowFocus: false,
} as const;

// Flows tables don't need to be live — refresh on user navigation is fine.
const TABLE_QUERY_OPTS = {
  staleTime: 1000 * 60 * 5,
  gcTime: 1000 * 60 * 30,
  placeholderData: keepPreviousData,
  retry: false,
  refetchOnWindowFocus: false,
} as const;

function windowParams(from?: string, to?: string): Record<string, string> {
  const p: Record<string, string> = {};
  if (from) p.start_time = from;
  if (to) p.end_time = to;
  return p;
}

function windowKey(from?: string, to?: string): string | undefined {
  if (!from && !to) return undefined;
  return `${from ?? ''}..${to ?? ''}`;
}

export function useNetworkSummary(
  deploymentId: string,
  opts?: { from?: string; to?: string; enabled?: boolean },
) {
  return useQuery({
    queryKey: networkKeys.summary(deploymentId, windowKey(opts?.from, opts?.to)),
    queryFn: () => api.getNetworkSummary(deploymentId, windowParams(opts?.from, opts?.to)),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...LIVE_QUERY_OPTS,
  });
}

export function useNetworkFlows(
  deploymentId: string,
  direction: NetworkDirection,
  opts?: {
    from?: string;
    to?: string;
    limit?: number;
    sort?: NetworkFlowsSort;
    enabled?: boolean;
  },
) {
  const params: Record<string, string> = {
    direction,
    ...windowParams(opts?.from, opts?.to),
  };
  if (opts?.limit !== undefined) params.limit = String(opts.limit);
  if (opts?.sort) params.sort = opts.sort;

  return useQuery({
    queryKey: networkKeys.flows(deploymentId, direction, windowKey(opts?.from, opts?.to), opts?.sort),
    queryFn: () => api.getNetworkFlows(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...TABLE_QUERY_OPTS,
  });
}

export function useNetworkTimeseries(
  deploymentId: string,
  direction: NetworkDirection,
  metric: NetworkMetric,
  opts?: {
    from?: string;
    to?: string;
    step?: string;
    groupBy?: NetworkGroupBy;
    enabled?: boolean;
  },
) {
  const params: Record<string, string> = {
    direction,
    metric,
    ...windowParams(opts?.from, opts?.to),
  };
  if (opts?.step) params.step = opts.step;
  if (opts?.groupBy) params.group_by = opts.groupBy;

  return useQuery({
    queryKey: networkKeys.timeseries(
      deploymentId,
      direction,
      metric,
      windowKey(opts?.from, opts?.to),
      opts?.step,
      opts?.groupBy,
    ),
    queryFn: () => api.getNetworkTimeseries(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    ...LIVE_QUERY_OPTS,
  });
}
