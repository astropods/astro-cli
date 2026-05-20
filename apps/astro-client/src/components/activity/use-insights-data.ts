import { useMemo } from "react";
import { useAccountActivitySummary, useBlueprintsSummary } from "@/api/queries/observability";
import { buildModelColorMap } from "./model-colors";
import type { AccountBlueprintsSummaryResponse, AccountObservabilitySummaryResponse } from "@/lib/api";

export const ALL_AGENTS_KEY = "__all__";
export const ALL_AGENTS_COLOR = "var(--color-indigo-500)";

type Blueprint = AccountBlueprintsSummaryResponse["blueprints"][number];

export function buildFilteredSummary(
  blueprints: Blueprint[],
  period: { start: string; end: string; days: number },
): AccountObservabilitySummaryResponse {
  // Single pass over blueprints: accumulate scalar totals, the per-(agent, date)
  // cost index used for the chart output, and per-date totals for sparklines.
  let totalCost = 0, totalReqs = 0, totalIn = 0, totalOut = 0;
  const dateSet = new Set<string>();
  const costIndex = new Map<string, Map<string, number>>();
  const totalsByDate = {
    cost: new Map<string, number>(),
    req:  new Map<string, number>(),
    tok:  new Map<string, number>(),
  };
  const addToDate = (map: Map<string, number>, date: string, v: number) => {
    map.set(date, (map.get(date) ?? 0) + v);
    dateSet.add(date);
  };

  for (const b of blueprints) {
    totalCost += b.cost_usd;
    totalReqs += b.requests;
    totalIn   += b.input_tokens;
    totalOut  += b.output_tokens;
    const perAgentCost = new Map<string, number>();
    for (const d of b.cost_over_time ?? [])     { perAgentCost.set(d.date, d.cost_usd); addToDate(totalsByDate.cost, d.date, d.cost_usd); }
    for (const d of b.requests_over_time ?? []) { addToDate(totalsByDate.req,  d.date, d.requests); }
    for (const d of b.tokens_over_time ?? [])   { addToDate(totalsByDate.tok,  d.date, d.input_tokens + d.output_tokens); }
    costIndex.set(b.agent_name, perAgentCost);
  }

  const allDates   = [...dateSet].sort();
  const costByDate = allDates.map((d) => totalsByDate.cost.get(d) ?? 0);
  const reqsByDate = allDates.map((d) => totalsByDate.req.get(d)  ?? 0);
  const toksByDate = allDates.map((d) => totalsByDate.tok.get(d)  ?? 0);

  const n    = Math.max(period.days, 1);
  const half = (arr: number[], s: number, e: number) => arr.slice(s, e).reduce((a, v) => a + v, 0);
  const pct  = (c: number, p: number) => (p > 0 ? parseFloat((((c - p) / p) * 100).toFixed(1)) : null);

  // Split by calendar midpoint so sparse agents don't skew the comparison.
  // Array-index splitting would compare 1 active day vs 2 active days for a
  // 30-day period where the agent was only active 3 days.
  const midTime = (new Date(period.start).getTime() + new Date(period.end).getTime()) / 2;
  const midDate = new Date(midTime).toISOString().slice(0, 10);
  const prevIdx = allDates.filter((d) => d < midDate).length;

  return {
    period,
    totals: {
      cost_usd: parseFloat(totalCost.toFixed(2)),
      requests: totalReqs,
      input_tokens: totalIn,
      output_tokens: totalOut,
      active_agents: blueprints.length,
    },
    daily_avg: {
      cost_usd: parseFloat((totalCost / n).toFixed(2)),
      requests: Math.round(totalReqs / n),
      tokens: Math.round((totalIn + totalOut) / n),
    },
    change: {
      cost_pct:     pct(half(costByDate, prevIdx, allDates.length), half(costByDate, 0, prevIdx)),
      requests_pct: pct(half(reqsByDate, prevIdx, allDates.length), half(reqsByDate, 0, prevIdx)),
      tokens_pct:   pct(half(toksByDate, prevIdx, allDates.length), half(toksByDate, 0, prevIdx)),
    },
    cost_over_time: allDates.map((date) => ({
      date,
      models: blueprints.map((b) => ({ model: b.agent_name, cost_usd: costIndex.get(b.agent_name)?.get(date) ?? 0 })),
    })),
    cost_by_model: [],
    sparklines: { cost: costByDate, requests: reqsByDate, tokens: toksByDate },
  };
}

interface UseInsightsDataOpts {
  account: string;
  selectedAgents: string[];
  // Caller passes from/to so the hook keys queries under the same window
  // the page primed the cache with.
  from?: string;
  to?: string;
}

export function useInsightsData({
  account,
  selectedAgents,
  from,
  to,
}: UseInsightsDataOpts) {
  const { data: summary, isLoading: summaryLoading } = useAccountActivitySummary(account, from, to);
  const { data: blueprintsData, isLoading: blueprintsLoading } = useBlueprintsSummary(account, from, to);

  const allAgentNames = useMemo(
    () => blueprintsData?.blueprints.map((b) => b.agent_name) ?? [],
    [blueprintsData],
  );

  const filteredBlueprints = useMemo(() => {
    const all = blueprintsData?.blueprints ?? [];
    if (selectedAgents.length > 0 && selectedAgents[0] !== ALL_AGENTS_KEY)
      return all.filter((b) => selectedAgents.includes(b.agent_name));
    return all;
  }, [blueprintsData, selectedAgents]);

  const chartBlueprints = useMemo(() => {
    if (selectedAgents.length > 0) return filteredBlueprints;
    return [...(blueprintsData?.blueprints ?? [])].sort((a, b) => b.cost_usd - a.cost_usd).slice(0, 5);
  }, [selectedAgents, filteredBlueprints, blueprintsData]);

  const agentCostOverTime = useMemo(() => {
    const isAll = selectedAgents[0] === ALL_AGENTS_KEY;
    const source = isAll ? (blueprintsData?.blueprints ?? []) : chartBlueprints;
    const allDates = [...new Set(source.flatMap((b) => b.cost_over_time?.map((d) => d.date) ?? []))].sort();
    const costIndex = new Map(source.map((b) => [b.agent_name, new Map(b.cost_over_time?.map((d) => [d.date, d.cost_usd]) ?? [])]));
    if (isAll) {
      return allDates.map((date) => ({
        date,
        models: [{ model: ALL_AGENTS_KEY, cost_usd: source.reduce((sum, b) => sum + (costIndex.get(b.agent_name)?.get(date) ?? 0), 0) }],
      }));
    }
    return allDates.map((date) => ({
      date,
      models: source.map((b) => ({ model: b.agent_name, cost_usd: costIndex.get(b.agent_name)?.get(date) ?? 0 })),
    }));
  }, [chartBlueprints, selectedAgents, blueprintsData]);

  const allAgentColorMap = useMemo(() => buildModelColorMap(allAgentNames), [allAgentNames]);

  const displaySummary = useMemo(() => {
    const isFiltered =
      selectedAgents.length > 0 && selectedAgents[0] !== ALL_AGENTS_KEY && blueprintsData?.period;
    if (isFiltered) return buildFilteredSummary(filteredBlueprints, blueprintsData.period);
    return summary;
  }, [filteredBlueprints, blueprintsData, selectedAgents, summary]);

  const activeColorMap = useMemo(
    () => ({ ...allAgentColorMap, [ALL_AGENTS_KEY]: ALL_AGENTS_COLOR }),
    [allAgentColorMap],
  );

  return {
    allAgentNames,
    filteredBlueprints,
    agentCostOverTime,
    displaySummary,
    allAgentColorMap,
    activeColorMap,
    isLoading: summaryLoading || blueprintsLoading,
    hasData: filteredBlueprints.some((b) => b.requests > 0),
  };
}
