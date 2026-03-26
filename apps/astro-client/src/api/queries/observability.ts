import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { observabilityKeys } from './keys';

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
