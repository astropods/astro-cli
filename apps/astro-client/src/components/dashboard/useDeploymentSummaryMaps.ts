import { useMemo } from "react";
import { useObservabilitySummaries } from "@/api/queries/observability";
import { formatRelativeTime } from "@/lib/deployment-utils";
import type { AgentDeploymentSummary } from "@/lib/api";

export function useDeploymentSummaryMaps(
  accountName: string,
  deployments: AgentDeploymentSummary[],
) {
  const { data: summariesData } = useObservabilitySummaries(accountName);
  return useMemo(() => {
    const requestCounts = new Map<string, number>();
    const lastActiveTimes = new Map<string, string>();
    const requestSeries = new Map<string, number[]>();
    const tokenSeries = new Map<string, number[]>();
    for (const d of deployments) {
      const entry = summariesData?.summaries[d.id];
      if (entry?.total_traces !== undefined) requestCounts.set(d.id, entry.total_traces);
      if (entry?.last_trace_at) lastActiveTimes.set(d.id, formatRelativeTime(entry.last_trace_at));
      if (entry?.request_series) requestSeries.set(d.id, entry.request_series);
      if (entry?.token_series) tokenSeries.set(d.id, entry.token_series);
    }
    return { requestCounts, lastActiveTimes, requestSeries, tokenSeries };
  }, [deployments, summariesData]);
}
