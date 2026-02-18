import { useQuery } from '@tanstack/react-query';
import { api } from '../../lib/api';
import { observabilityKeys } from './keys';

export function useObservabilityMetrics(
  account: string,
  name: string,
  params?: Record<string, string>,
  enabled = true,
) {
  return useQuery({
    queryKey: observabilityKeys.metrics(account, name, params),
    queryFn: () => api.getObservabilityMetrics(account, name, params),
    enabled: !!account && !!name && enabled,
    refetchInterval: 60_000,
  });
}

export function useObservabilitySummary(
  account: string,
  name: string,
  params?: Record<string, string>,
  enabled = true,
) {
  return useQuery({
    queryKey: observabilityKeys.summary(account, name, params),
    queryFn: () => api.getObservabilitySummary(account, name, params),
    enabled: !!account && !!name && enabled,
    refetchInterval: 60_000,
  });
}

export function useObservabilityTraces(
  account: string,
  name: string,
  params?: Record<string, string>,
  enabled = true,
) {
  return useQuery({
    queryKey: observabilityKeys.traces(account, name, params),
    queryFn: () => api.getObservabilityTraces(account, name, params),
    enabled: !!account && !!name && enabled,
  });
}
