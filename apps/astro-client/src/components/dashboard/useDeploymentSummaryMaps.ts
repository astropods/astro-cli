import { useMemo } from "react";
import {
  useObservabilitySummaries,
  useObservabilitySummariesForAccounts,
} from "@/api/queries/observability";
import type { AgentDeploymentSummary, DeploymentSummaryEntry } from "@/lib/api";

function buildSummaryMaps(
  deployments: AgentDeploymentSummary[],
  summaries: Record<string, DeploymentSummaryEntry> | undefined,
) {
  const requestCounts = new Map<string, number>();
  const requestSeries = new Map<string, number[]>();
  const tokenSeries = new Map<string, number[]>();
  for (const d of deployments) {
    const entry = summaries?.[d.id];
    if (entry?.total_traces !== undefined) requestCounts.set(d.id, entry.total_traces);
    if (entry?.request_series) requestSeries.set(d.id, entry.request_series);
    if (entry?.token_series) tokenSeries.set(d.id, entry.token_series);
  }
  return { requestCounts, requestSeries, tokenSeries };
}

export function useDeploymentSummaryMaps(
  accountName: string,
  deployments: AgentDeploymentSummary[],
) {
  const { data: summariesData } = useObservabilitySummaries(accountName);
  return useMemo(
    () => buildSummaryMaps(deployments, summariesData?.summaries),
    [deployments, summariesData],
  );
}

// Cross-account variant for the all-accounts Agents view: fans out summaries
// per owning account and merges them before building the id-keyed maps.
export function useMultiAccountDeploymentSummaryMaps(
  deployments: (AgentDeploymentSummary & { account: string })[],
) {
  const accountNames = useMemo(
    () => Array.from(new Set(deployments.map((d) => d.account))),
    [deployments],
  );
  const { summaries } = useObservabilitySummariesForAccounts(accountNames);
  return useMemo(() => buildSummaryMaps(deployments, summaries), [deployments, summaries]);
}
