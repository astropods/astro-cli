import { keepPreviousData, useQueries, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { observabilityKeys } from './keys';

export function useAccountObservabilitySummary(
  account: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.accountSummary(account, opts?.window),
    queryFn: () => api.getAccountObservabilitySummary(account, params),
    enabled: (opts?.enabled ?? true) && !!account,
    staleTime: 0,
    retry: false,
  });
}

export function useObservabilityMetrics(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.metrics(deploymentId, opts?.window),
    queryFn: () => api.getObservabilityMetrics(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    staleTime: 0,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
}

export function useObservabilitySummaries(deploymentIds: string[]) {
  return useQueries({
    queries: deploymentIds.map((id) => ({
      queryKey: observabilityKeys.summary(id),
      queryFn: () => api.getObservabilitySummary(id),
      enabled: !!id,
      staleTime: 0,
      gcTime: 1000 * 60 * 30,
      placeholderData: keepPreviousData,
      retry: false,
      refetchOnWindowFocus: false,
    })),
  });
}

export function useObservabilitySummary(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.summary(deploymentId, opts?.window),
    queryFn: () => api.getObservabilitySummary(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    staleTime: 0,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
}

export function useObservabilityTraces(
  deploymentId: string,
  params?: Record<string, string>,
  opts?: { enabled?: boolean; window?: string },
) {
  return useQuery({
    queryKey: observabilityKeys.traces(deploymentId, opts?.window),
    queryFn: () => api.getObservabilityTraces(deploymentId, params),
    enabled: (opts?.enabled ?? true) && !!deploymentId,
    staleTime: 0,
    gcTime: 1000 * 60 * 30,
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
}
