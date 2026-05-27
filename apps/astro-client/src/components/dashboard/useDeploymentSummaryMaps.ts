import { useMemo } from "react";
import { useObservabilitySummaries } from "@/api/queries/observability";
import { formatRelativeTime } from "@/lib/deployment-utils";
import type { AgentDeployment } from "@/lib/api";

export function useDeploymentSummaryMaps(
  accountName: string,
  deployments: AgentDeployment[],
) {
  const { data: summariesData } = useObservabilitySummaries(accountName);
  return useMemo(() => {
    const requestCounts = new Map<string, number>();
    const lastActiveTimes = new Map<string, string>();
    for (const d of deployments) {
      const entry = summariesData?.summaries[d.id];
      if (entry?.total_traces !== undefined) requestCounts.set(d.id, entry.total_traces);
      if (entry?.last_trace_at) lastActiveTimes.set(d.id, formatRelativeTime(entry.last_trace_at));
    }
    return { requestCounts, lastActiveTimes };
  }, [deployments, summariesData]);
}
