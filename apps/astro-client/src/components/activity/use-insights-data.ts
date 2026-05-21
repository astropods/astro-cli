import { useMemo } from "react";
import {
  useAccountActivitySummary,
  useBlueprintsSummary,
  useUsersSummary,
} from "@/api/queries/observability";
import { useAccountMembers } from "@/api/queries/accounts";
import { buildPeriodParams, type ActivityRange } from "./ranges";
import { buildModelColorMap } from "./model-colors";
import { ALL_USERS_KEY, UNATTRIBUTED_USER_KEY, UNAUTHORIZED_USER_KEY, classifyUserId } from "./user-classification";
import type {
  AccountBlueprintsSummaryResponse,
  AccountObservabilitySummaryResponse,
  AccountUsersSummaryResponse,
} from "@/lib/api";

export const ALL_AGENTS_KEY = "__all__";
export const ALL_AGENTS_COLOR = "var(--color-indigo-500)";

/** Both insights hooks accept `enabled` (default true) so a future caller
 *  that mounts both tabs simultaneously can gate the inactive view's
 *  Langfuse-backed queries. */
function useResolvedPeriod(range: ActivityRange, ssrFrom?: string | null, ssrTo?: string | null) {
  const { from: computedFrom, to: computedTo } = useMemo(() => buildPeriodParams(range), [range]);
  return { from: ssrFrom ?? computedFrom, to: ssrTo ?? computedTo };
}

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

  let totalTok = 0;
  for (const b of blueprints) {
    totalCost += b.cost_usd;
    totalReqs += b.requests;
    totalIn   += b.input_tokens;
    totalOut  += b.output_tokens;
    totalTok  += b.total_tokens;
    const perAgentCost = new Map<string, number>();
    for (const d of b.cost_over_time ?? [])     { perAgentCost.set(d.date, d.cost_usd); addToDate(totalsByDate.cost, d.date, d.cost_usd); }
    for (const d of b.requests_over_time ?? []) { addToDate(totalsByDate.req,  d.date, d.requests); }
    for (const d of b.tokens_over_time ?? [])   { addToDate(totalsByDate.tok,  d.date, d.total_tokens); }
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
  //
  // The all-time range omits period.start/end on the server side, so the
  // Date math below would yield NaN and toISOString would throw. In that
  // case there's no meaningful prior window to compare against — leave
  // prevIdx at 0 so half(... 0, prevIdx) is 0 and change% comes out as null.
  const startMs = new Date(period.start || "").getTime();
  const endMs = new Date(period.end || "").getTime();
  const haveBounds = Number.isFinite(startMs) && Number.isFinite(endMs);
  let prevIdx = 0;
  if (haveBounds) {
    const midDate = new Date((startMs + endMs) / 2).toISOString().slice(0, 10);
    prevIdx = allDates.filter((d) => d < midDate).length;
  }

  return {
    period,
    totals: {
      cost_usd: parseFloat(totalCost.toFixed(2)),
      requests: totalReqs,
      input_tokens: totalIn,
      output_tokens: totalOut,
      total_tokens: totalTok,
      active_agents: blueprints.length,
    },
    daily_avg: {
      cost_usd: parseFloat((totalCost / n).toFixed(2)),
      requests: Math.round(totalReqs / n),
      tokens: Math.round(totalTok / n),
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
  range: ActivityRange;
  selectedAgents: string[];
  ssrFrom?: string | null;
  ssrTo?: string | null;
  enabled?: boolean;
}

export function useInsightsData({
  account,
  range,
  selectedAgents,
  ssrFrom,
  ssrTo,
  enabled = true,
}: UseInsightsDataOpts) {
  const { from, to } = useResolvedPeriod(range, ssrFrom, ssrTo);

  const summaryQ = useAccountActivitySummary(account, from, to, { enabled });
  const blueprintsQ = useBlueprintsSummary(account, from, to, { enabled });

  const summary = summaryQ.data;
  const summaryLoading = summaryQ.isLoading;
  const blueprintsData = blueprintsQ.data;
  const blueprintsLoading = blueprintsQ.isLoading;

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
    const isAll = selectedAgents.length > 0 && selectedAgents[0] === ALL_AGENTS_KEY;
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
    from,
    to,
    allAgentNames,
    filteredBlueprints,
    agentCostOverTime,
    displaySummary,
    allAgentColorMap,
    activeColorMap,
    summaryLoading,
    blueprintsLoading,
    isLoading: summaryLoading || blueprintsLoading,
    hasData: filteredBlueprints.some((b) => b.requests > 0),
  };
}

// ── Users-view data hook ─────────────────────────────────────────────────────

type UserRow = AccountUsersSummaryResponse["users"][number];

interface UseUsersInsightsDataOpts {
  account: string;
  range: ActivityRange;
  selectedUsers: string[];
  ssrFrom?: string | null;
  ssrTo?: string | null;
  enabled?: boolean;
}

function buildUserCostOverTime(
  summary: AccountObservabilitySummaryResponse | undefined,
  visibleUserIds: string[],
  memberIds: Set<string>,
): Array<{ date: string; models: Array<{ model: string; cost_usd: number }> }> {
  const rows = summary?.cost_over_time_by_user ?? [];
  if (rows.length === 0 || visibleUserIds.length === 0) return [];
  // An empty visibleUserIds set means "nothing to show yet" (usersData still
  // loading, or empty period) — NOT "show every user." Treating it as "all"
  // would flash every user before the top-5 filter resolves.
  const visible = new Set(visibleUserIds);
  return rows.map((r) => {
    const byKey = new Map<string, number>();
    for (const u of r.users) {
      const key = classifyUserId(u.user_id, memberIds);
      byKey.set(key, (byKey.get(key) ?? 0) + u.cost_usd);
    }
    return {
      date: r.date,
      models: [...byKey.entries()]
        .filter(([key]) => visible.has(key))
        .map(([model, cost_usd]) => ({ model, cost_usd })),
    };
  });
}

function recomputeTotalsFromUsers(
  users: UserRow[],
  period: { start: string; end: string; days: number },
): AccountObservabilitySummaryResponse {
  const totalCost   = users.reduce((s, u) => s + u.cost_usd, 0);
  const totalReqs   = users.reduce((s, u) => s + u.requests, 0);
  const totalTokens = users.reduce((s, u) => s + u.tokens, 0);
  const n = Math.max(period.days, 1);
  return {
    period,
    totals: {
      cost_usd: parseFloat(totalCost.toFixed(2)),
      requests: totalReqs,
      // Users view derives tokens from the traces view, which only exposes
      // the combined sum. total_tokens is the canonical field; input/output
      // stay at 0 here and consumers should read total_tokens.
      input_tokens: 0,
      output_tokens: 0,
      total_tokens: totalTokens,
      active_agents: 0,
    },
    daily_avg: {
      cost_usd: parseFloat((totalCost / n).toFixed(2)),
      requests: Math.round(totalReqs / n),
      tokens: Math.round(totalTokens / n),
    },
    change: null,
    cost_over_time: [],
    cost_by_model: [],
    sparklines: { cost: [], requests: [], tokens: [] },
  };
}

export function useUsersInsightsData({
  account,
  range,
  selectedUsers,
  ssrFrom,
  ssrTo,
  enabled = true,
}: UseUsersInsightsDataOpts) {
  const { from, to } = useResolvedPeriod(range, ssrFrom, ssrTo);

  const summaryQ = useAccountActivitySummary(account, from, to, { groupBy: "user", enabled });
  const usersQ = useUsersSummary(account, from, to, { enabled });
  // Members query is intentionally NOT gated by `enabled` — it's cached app-wide
  // for avatar/badge resolution and worth keeping warm even when this view is idle.
  const membersQ = useAccountMembers(account);

  const summary = summaryQ.data;
  const summaryLoading = summaryQ.isLoading;
  const usersData = usersQ.data;
  const usersLoading = usersQ.isLoading;
  const membersData = membersQ.data;
  const membersLoading = membersQ.isLoading;

  const memberIds = useMemo(
    () => new Set(membersData?.members.map((m) => m.user_id) ?? []),
    [membersData],
  );

  const allUserIds = useMemo(() => {
    const ids = new Set<string>();
    for (const u of usersData?.users ?? []) ids.add(classifyUserId(u.user_id, memberIds));
    for (const row of summary?.cost_over_time_by_user ?? []) {
      for (const u of row.users) ids.add(classifyUserId(u.user_id, memberIds));
    }
    return [...ids];
  }, [usersData, summary, memberIds]);

  const allUserColorMap = useMemo(() => buildModelColorMap(allUserIds), [allUserIds]);

  const userLabelMap = useMemo(() => {
    const map: Record<string, string> = {
      [UNATTRIBUTED_USER_KEY]: "Unattributed",
      [UNAUTHORIZED_USER_KEY]: "Unauthorized",
      [ALL_USERS_KEY]: "All Users",
    };
    const memberById = new Map(membersData?.members.map((m) => [m.user_id, m]) ?? []);
    for (const uid of allUserIds) {
      if (uid === UNATTRIBUTED_USER_KEY || uid === UNAUTHORIZED_USER_KEY) continue;
      const m = memberById.get(uid);
      map[uid] = m ? (m.display_name || m.username) : uid;
    }
    return map;
  }, [allUserIds, membersData]);

  const isAllSelected = selectedUsers.length > 0 && selectedUsers[0] === ALL_USERS_KEY;
  const noSelection = selectedUsers.length === 0;

  const filteredUsers = useMemo(() => {
    const all = usersData?.users ?? [];
    if (noSelection || isAllSelected) return all;
    return all.filter((u) => selectedUsers.includes(classifyUserId(u.user_id, memberIds)));
  }, [usersData, selectedUsers, noSelection, isAllSelected, memberIds]);

  const chartVisibleUserIds = useMemo(() => {
    if (noSelection || isAllSelected) {
      const byKey = new Map<string, number>();
      for (const u of usersData?.users ?? []) {
        const key = classifyUserId(u.user_id, memberIds);
        byKey.set(key, (byKey.get(key) ?? 0) + u.cost_usd);
      }
      return [...byKey.entries()]
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5)
        .map(([k]) => k);
    }
    return selectedUsers;
  }, [usersData, selectedUsers, noSelection, isAllSelected, memberIds]);

  const userCostOverTime = useMemo(
    () => buildUserCostOverTime(summary, chartVisibleUserIds, memberIds),
    [summary, chartVisibleUserIds, memberIds],
  );

  const displaySummary = useMemo(() => {
    const isFiltered = !noSelection && !isAllSelected && usersData?.period;
    if (isFiltered) return recomputeTotalsFromUsers(filteredUsers, usersData.period);
    return summary;
  }, [filteredUsers, usersData, summary, noSelection, isAllSelected]);

  const activeColorMap = useMemo(
    () => ({ ...allUserColorMap, [ALL_USERS_KEY]: "var(--color-indigo-500)" }),
    [allUserColorMap],
  );

  return {
    from,
    to,
    allUserIds,
    filteredUsers,
    userCostOverTime,
    displaySummary,
    allUserColorMap,
    activeColorMap,
    userLabelMap,
    summaryLoading,
    // usersLoading reports both the users-summary endpoint AND the members
    // query — classification depends on both, so until members lands every
    // named user would be misclassified as Unauthorized.
    usersLoading: usersLoading || membersLoading,
    isLoading: summaryLoading || usersLoading || membersLoading,
    hasData: (usersData?.users ?? []).some((u) => u.requests > 0),
  };
}
