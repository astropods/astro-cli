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

type Blueprint = AccountBlueprintsSummaryResponse["blueprints"][number];

// emptyAccountSummary is the zero-state both insights hooks fall back to
// before their underlying data fetch resolves. Mirrors the server's response
// shape so consumers (StatCards / sparklines / change tile) don't need
// undefined-handling for the cold path. Shared const — read-only by
// convention; nothing in the data-flow mutates it.
const emptyAccountSummary: AccountObservabilitySummaryResponse = {
  period: { start: "", end: "", days: 0 },
  totals: { cost_usd: 0, requests: 0, input_tokens: 0, output_tokens: 0, total_tokens: 0, active_agents: 0 },
  daily_avg: { cost_usd: 0, requests: 0, tokens: 0 },
  change: null,
  cost_over_time: [],
  cost_by_model: [],
  sparklines: { cost: [], requests: [], tokens: [] },
};

// sliceBlueprintByWindow rebuilds a single Blueprint's totals + over_time
// arrays from a UTC day window [fromDate, toDate] inclusive. Each blueprint
// from the server already carries per-day data for cost/requests/tokens, so
// every range toggle is a pure JS computation against in-memory data — no
// network round-trip.
//
// Regression notice — p95_latency_ms and top_model USED to be range-scoped:
// before client-side slicing the server forwarded from/to to Langfuse and
// these two fields reflected the selected range. With client-side slicing
// the client queries all-time and we can't reproduce these locally (no
// per-day latency distribution, no per-(day, model) breakdown on the
// blueprint shape). They now stay at the server's all-time values — a
// deliberate trade-off accepted in exchange for instant toggles.
export function sliceBlueprintByWindow(b: Blueprint, fromDate: string, toDate: string): Blueprint {
  const inRange = (d: { date: string }) => d.date >= fromDate && d.date <= toDate;
  const cost_over_time = (b.cost_over_time ?? []).filter(inRange);
  const requests_over_time = (b.requests_over_time ?? []).filter(inRange);
  const tokens_over_time = (b.tokens_over_time ?? []).filter(inRange);
  const cost_usd = cost_over_time.reduce((s, d) => s + d.cost_usd, 0);
  const requests = requests_over_time.reduce((s, d) => s + d.requests, 0);
  const input_tokens = tokens_over_time.reduce((s, d) => s + d.input_tokens, 0);
  const output_tokens = tokens_over_time.reduce((s, d) => s + d.output_tokens, 0);
  const total_tokens = tokens_over_time.reduce((s, d) => s + d.total_tokens, 0);
  return {
    ...b,
    cost_over_time,
    requests_over_time,
    tokens_over_time,
    cost_usd: parseFloat(cost_usd.toFixed(4)),
    requests,
    input_tokens,
    output_tokens,
    total_tokens,
    cost_per_request: requests > 0 ? parseFloat((cost_usd / requests).toFixed(4)) : 0,
    tok_per_request: requests > 0 ? parseFloat((total_tokens / requests).toFixed(1)) : 0,
    // p95_latency_ms + top_model + users_used stay as server values (all-time).
  };
}

// sliceBlueprintsResponseByRange returns a synthetic response derived from the
// all-time response, with every blueprint sliced to the URL window. Returns
// the input unchanged for the all-time range.
export function sliceBlueprintsResponseByRange(
  response: AccountBlueprintsSummaryResponse | undefined,
  range: ActivityRange,
): AccountBlueprintsSummaryResponse | undefined {
  if (!response) return response;
  if (range === "all") return response;
  const { from, to } = buildPeriodParams(range);
  const fromDate = (from ?? "").slice(0, 10);
  const toDate = (to ?? "").slice(0, 10);
  if (!fromDate || !toDate) return response;
  const days = Math.max(1, Math.round((Date.parse(`${toDate}T00:00:00Z`) - Date.parse(`${fromDate}T00:00:00Z`)) / 86_400_000) + 1);
  return {
    blueprints: response.blueprints.map((b) => sliceBlueprintByWindow(b, fromDate, toDate)),
    period: { start: from!, end: to!, days },
  };
}

// enumerateDates returns "YYYY-MM-DD" UTC strings from fromTs through toTs
// inclusive. The server emits per-day arrays only for days with activity, so
// callers stitching daily values across blueprints need to walk the bounded
// period independently to avoid gaps in the chart's x-axis (e.g. /insights
// ?range=30d showing May 7 as the earliest bar because nothing happened
// before that). Returns [] for the all-time case where the server omits
// period.start/end.
function enumerateDates(fromTs: string | undefined | null, toTs: string | undefined | null): string[] {
  const from = (fromTs ?? "").slice(0, 10);
  const to = (toTs ?? "").slice(0, 10);
  if (!from || !to || from > to) return [];
  const out: string[] = [];
  const d = new Date(`${from}T00:00:00Z`);
  const endMs = new Date(`${to}T00:00:00Z`).getTime();
  // Guard against malformed inputs that would loop forever.
  if (!Number.isFinite(endMs) || Number.isNaN(d.getTime())) return [];
  while (d.getTime() <= endMs) {
    out.push(d.toISOString().slice(0, 10));
    d.setUTCDate(d.getUTCDate() + 1);
  }
  return out;
}

export type ChangeTotals = { cost: number; requests: number; tokens: number };

// computeChange compares the current window's totals to the prior period's
// totals — same semantic the server-side `change` used to deliver: "this
// week vs last week", not "first half vs second half of this week". Prior
// totals are computed from the all-time data we already hold client-side,
// so this stays a pure JS operation (no extra round-trip on toggles).
// Returns null when `prior` is null (all-time range or unknown prior data).
function computeChange(
  current: ChangeTotals,
  prior: ChangeTotals | null,
): { cost_pct: number | null; requests_pct: number | null; tokens_pct: number | null } | null {
  if (!prior) return null;
  const pct = (c: number, p: number) => (p > 0 ? parseFloat((((c - p) / p) * 100).toFixed(1)) : null);
  return {
    cost_pct:     pct(current.cost,     prior.cost),
    requests_pct: pct(current.requests, prior.requests),
    tokens_pct:   pct(current.tokens,   prior.tokens),
  };
}

// shiftPriorWindow returns the same-length window immediately before
// [fromDate, toDate] inclusive, so a 7d range [Jan 4..Jan 10] yields the
// prior [Dec 28..Jan 3]. Returns null if either input is malformed.
export function shiftPriorWindow(
  fromDate: string,
  toDate: string,
): { priorFrom: string; priorTo: string } | null {
  const fromMs = Date.parse(`${fromDate}T00:00:00Z`);
  const toMs = Date.parse(`${toDate}T00:00:00Z`);
  if (!Number.isFinite(fromMs) || !Number.isFinite(toMs) || fromMs > toMs) return null;
  const lenDays = Math.round((toMs - fromMs) / 86_400_000) + 1;
  const priorToMs = fromMs - 86_400_000;
  const priorFromMs = priorToMs - (lenDays - 1) * 86_400_000;
  return {
    priorFrom: new Date(priorFromMs).toISOString().slice(0, 10),
    priorTo: new Date(priorToMs).toISOString().slice(0, 10),
  };
}

// sumBlueprintsWindow returns the [cost, requests, tokens] totals for a
// UTC day window across the supplied blueprints. Drives prior-period
// change% in the agents view — must be called against ALL-TIME blueprints,
// not range-sliced ones, so the prior days are still present.
function sumBlueprintsWindow(
  blueprints: Blueprint[],
  fromDate: string,
  toDate: string,
): ChangeTotals {
  let cost = 0, requests = 0, tokens = 0;
  for (const b of blueprints) {
    for (const d of b.cost_over_time ?? []) {
      if (d.date >= fromDate && d.date <= toDate) cost += d.cost_usd;
    }
    for (const d of b.requests_over_time ?? []) {
      if (d.date >= fromDate && d.date <= toDate) requests += d.requests;
    }
    for (const d of b.tokens_over_time ?? []) {
      if (d.date >= fromDate && d.date <= toDate) tokens += d.total_tokens;
    }
  }
  return { cost, requests, tokens };
}

export function buildFilteredSummary(
  blueprints: Blueprint[],
  period: { start: string; end: string; days: number },
  prior: ChangeTotals | null = null,
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

  // Bounded periods walk the full day range so the chart x-axis + sparklines
  // span every day in the window (zero-fill for inactive days), not just the
  // days the server returned data for. All-time falls back to the union of
  // dates because there's no bounded range to enumerate.
  const enumerated = enumerateDates(period.start, period.end);
  const allDates   = enumerated.length > 0 ? enumerated : [...dateSet].sort();
  const sparklines = {
    cost:     allDates.map((d) => totalsByDate.cost.get(d) ?? 0),
    requests: allDates.map((d) => totalsByDate.req.get(d)  ?? 0),
    tokens:   allDates.map((d) => totalsByDate.tok.get(d)  ?? 0),
  };

  const n = Math.max(period.days, 1);

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
    change: computeChange(
      { cost: totalCost, requests: totalReqs, tokens: totalTok },
      prior,
    ),
    cost_over_time: allDates.map((date) => ({
      date,
      models: blueprints.map((b) => ({ model: b.agent_name, cost_usd: costIndex.get(b.agent_name)?.get(date) ?? 0 })),
    })),
    cost_by_model: [],
    sparklines,
  };
}

interface UseInsightsDataOpts {
  account: string;
  range: ActivityRange;
  selectedAgents: string[];
  enabled?: boolean;
}

export function useInsightsData({
  account,
  range,
  selectedAgents,
  enabled = true,
}: UseInsightsDataOpts) {
  // Always fetch all-time blueprints; everything below derives from that
  // single source via client-side slicing. The account summary endpoint
  // used to be a fallback for displaySummary — dropped because the
  // blueprints data carries everything we need and the fallback path is
  // now a static zero-state.
  const blueprintsQ = useBlueprintsSummary(account, undefined, undefined, { enabled });

  const blueprintsAll = blueprintsQ.data;
  const blueprintsLoading = blueprintsQ.isLoading;

  // Slice the all-time response down to the URL range. Every memo downstream
  // reads from `blueprintsData` so the URL range is the only thing they need
  // to react to.
  const blueprintsData = useMemo(
    () => sliceBlueprintsResponseByRange(blueprintsAll, range),
    [blueprintsAll, range],
  );

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
    // Bounded periods enumerate every day in the window — server returns sparse
    // per-blueprint cost_over_time (only days with activity), so without this
    // the chart's first bar is the first day anything ran, not the first day
    // of the requested range.
    const enumerated = enumerateDates(blueprintsData?.period?.start, blueprintsData?.period?.end);
    const unionDates = [...new Set(source.flatMap((b) => b.cost_over_time?.map((d) => d.date) ?? []))].sort();
    const allDates = enumerated.length > 0 ? enumerated : unionDates;
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

  // Prior-period totals (this-week vs last-week semantic). Computed from
  // the all-time blueprints we already hold, filtered the same way as the
  // current view, so the StatCards change% badge stays a pure JS op.
  const priorTotals = useMemo<ChangeTotals | null>(() => {
    if (!blueprintsAll || range === "all") return null;
    const { fromDate, toDate } = rangeWindow(range);
    if (!fromDate || !toDate) return null;
    const prior = shiftPriorWindow(fromDate, toDate);
    if (!prior) return null;
    const all = blueprintsAll.blueprints;
    const filtered =
      selectedAgents.length > 0 && selectedAgents[0] !== ALL_AGENTS_KEY
        ? all.filter((b) => selectedAgents.includes(b.agent_name))
        : all;
    return sumBlueprintsWindow(filtered, prior.priorFrom, prior.priorTo);
  }, [blueprintsAll, range, selectedAgents]);

  const displaySummary = useMemo(() => {
    // The headline cards / sparklines derive from the SLICED blueprints so
    // the totals refresh instantly on a range toggle. Pre-fetch state is
    // a zero-state response (not the server summary, which we no longer
    // fetch here) — StatCards already handles undefined/zero gracefully.
    if (!blueprintsData?.period) return emptyAccountSummary;
    return buildFilteredSummary(filteredBlueprints, blueprintsData.period, priorTotals);
  }, [filteredBlueprints, blueprintsData, priorTotals]);

  const activeColorMap = useMemo(
    () => ({ ...allAgentColorMap, [ALL_AGENTS_KEY]: ALL_AGENTS_COLOR }),
    [allAgentColorMap],
  );

  return {
    from: blueprintsData?.period?.start,
    to: blueprintsData?.period?.end,
    allAgentNames,
    filteredBlueprints,
    agentCostOverTime,
    displaySummary,
    allAgentColorMap,
    activeColorMap,
    blueprintsLoading,
    isLoading: blueprintsLoading,
    hasData: filteredBlueprints.some((b) => b.requests > 0),
  };
}

// ── Users-view data hook ─────────────────────────────────────────────────────

type UserRow = AccountUsersSummaryResponse["users"][number];

interface UseUsersInsightsDataOpts {
  account: string;
  range: ActivityRange;
  selectedUsers: string[];
  enabled?: boolean;
}

// rangeWindow computes the UTC day window the URL range maps to. Returns
// empty strings for the all-time range (caller falls back to union of dates).
function rangeWindow(range: ActivityRange): { fromDate: string; toDate: string } {
  if (range === "all") return { fromDate: "", toDate: "" };
  const { from, to } = buildPeriodParams(range);
  return { fromDate: (from ?? "").slice(0, 10), toDate: (to ?? "").slice(0, 10) };
}

function buildUserCostOverTime(
  summary: AccountObservabilitySummaryResponse | undefined,
  range: ActivityRange,
  visibleUserIds: string[],
  memberIds: Set<string>,
): Array<{ date: string; models: Array<{ model: string; cost_usd: number }> }> {
  const rows = summary?.cost_over_time_by_user ?? [];
  if (visibleUserIds.length === 0) return [];
  // An empty visibleUserIds set means "nothing to show yet" (usersData still
  // loading, or empty period) — NOT "show every user." Treating it as "all"
  // would flash every user before the top-5 filter resolves.
  const visible = new Set(visibleUserIds);

  // Index server rows by their UTC day. We always pull all-time from the
  // server and slice client-side here, so range toggles are instant.
  const rowsByDate = new Map(rows.map((r) => [r.date.slice(0, 10), r]));
  const { fromDate, toDate } = rangeWindow(range);
  const enumerated = fromDate && toDate ? enumerateDates(fromDate, toDate) : [];
  const dates = enumerated.length > 0 ? enumerated : rows.map((r) => r.date);

  return dates.map((date) => {
    const row = rowsByDate.get(date);
    const byKey = new Map<string, number>();
    for (const u of row?.users ?? []) {
      const key = classifyUserId(u.user_id, memberIds);
      byKey.set(key, (byKey.get(key) ?? 0) + u.cost_usd);
    }
    return {
      date,
      models: [...byKey.entries()]
        .filter(([key]) => visible.has(key))
        .map(([model, cost_usd]) => ({ model, cost_usd })),
    };
  });
}

// sliceUsersByRange walks the per-(day, user) data from the all-time summary
// response ONCE and returns both shapes the users view needs:
//   - users: per-user totals scoped to the URL range (drives the Top Spenders
//     table). Falls back to the all-time array for the all-time range, when
//     the per-day data is absent, or on any older-server response missing
//     cost_over_time_by_user.
//   - sparklines: per-day flat totals aligned to the bounded window length,
//     optionally filtered by visibleUserIds (drives the stat-card mini bars).
//
// agents_used is sourced from the all-time users response — it's not
// sliceable from per-(day, user) data and the chip stack staying all-time
// is a known small degradation.
export function sliceUsersByRange(
  summary: AccountObservabilitySummaryResponse | undefined,
  usersData: AccountUsersSummaryResponse | undefined,
  range: ActivityRange,
  visibleUserIds: Set<string> | null = null,
  memberIds: Set<string> = new Set(),
): {
  users: UserRow[];
  sparklines: { cost: number[]; requests: number[]; tokens: number[] };
} {
  const rows = summary?.cost_over_time_by_user ?? [];
  const agentsByUser = new Map((usersData?.users ?? []).map((u) => [u.user_id, u.agents_used]));
  const { fromDate, toDate } = rangeWindow(range);
  const bounded = !!fromDate && !!toDate;

  // No per-day data → no sparklines, fall back to all-time users.
  if (rows.length === 0) {
    return { users: usersData?.users ?? [], sparklines: { cost: [], requests: [], tokens: [] } };
  }

  const perUser = new Map<string, { cost: number; requests: number; tokens: number; lastSeen: string }>();
  const dailyTotals = new Map<string, { cost: number; requests: number; tokens: number }>();

  for (const row of rows) {
    const date = row.date.slice(0, 10);
    if (bounded && (date < fromDate || date > toDate)) continue;

    for (const u of row.users) {
      // Per-user aggregation (skipped for the all-time path; we pass the
      // server's users array through verbatim there).
      if (bounded) {
        const existing = perUser.get(u.user_id) ?? { cost: 0, requests: 0, tokens: 0, lastSeen: "" };
        existing.cost += u.cost_usd;
        existing.requests += u.requests;
        existing.tokens += u.tokens;
        // last_seen tracks the most recent active day in the window. Day-level
        // precision; hour-level is only available from the per-range server query.
        if ((u.requests > 0 || u.cost_usd > 0) && date > existing.lastSeen) {
          existing.lastSeen = date;
        }
        perUser.set(u.user_id, existing);
      }

      // Per-day sparkline aggregation. Filter by visibleUserIds when one is
      // active so an applied user chip narrows the headline cards' bars.
      if (visibleUserIds && !visibleUserIds.has(classifyUserId(u.user_id, memberIds))) continue;
      const dayAcc = dailyTotals.get(date) ?? { cost: 0, requests: 0, tokens: 0 };
      dayAcc.cost += u.cost_usd;
      dayAcc.requests += u.requests;
      dayAcc.tokens += u.tokens;
      dailyTotals.set(date, dayAcc);
    }
  }

  const users: UserRow[] = bounded
    ? [...perUser.entries()].map(([user_id, agg]) => ({
        user_id,
        cost_usd: parseFloat(agg.cost.toFixed(4)),
        requests: agg.requests,
        tokens: agg.tokens,
        last_seen: agg.lastSeen ? `${agg.lastSeen}T00:00:00Z` : undefined,
        agents_used: agentsByUser.get(user_id) ?? [],
      }))
    : (usersData?.users ?? []);

  const dates = bounded ? enumerateDates(fromDate, toDate) : rows.map((r) => r.date.slice(0, 10));
  const sparklines = {
    cost: dates.map((d) => parseFloat((dailyTotals.get(d)?.cost ?? 0).toFixed(4))),
    requests: dates.map((d) => dailyTotals.get(d)?.requests ?? 0),
    tokens: dates.map((d) => dailyTotals.get(d)?.tokens ?? 0),
  };

  return { users, sparklines };
}

export function recomputeTotalsFromUsers(
  users: UserRow[],
  period: { start: string; end: string; days: number },
  sparklines: { cost: number[]; requests: number[]; tokens: number[] } = { cost: [], requests: [], tokens: [] },
  prior: ChangeTotals | null = null,
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
    change: computeChange(
      { cost: totalCost, requests: totalReqs, tokens: totalTokens },
      prior,
    ),
    cost_over_time: [],
    cost_by_model: [],
    sparklines,
  };
}

// sumUsersWindow returns prior-period totals for the users view. Walks
// the all-time per-(day, user) summary, scoped to the supplied window
// and to the same user filter the current view applies.
function sumUsersWindow(
  summary: AccountObservabilitySummaryResponse | undefined,
  fromDate: string,
  toDate: string,
  visibleUserIds: Set<string> | null,
  memberIds: Set<string>,
): ChangeTotals {
  let cost = 0, requests = 0, tokens = 0;
  for (const row of summary?.cost_over_time_by_user ?? []) {
    const date = row.date.slice(0, 10);
    if (date < fromDate || date > toDate) continue;
    for (const u of row.users) {
      if (visibleUserIds && !visibleUserIds.has(classifyUserId(u.user_id, memberIds))) continue;
      cost += u.cost_usd;
      requests += u.requests;
      tokens += u.tokens;
    }
  }
  return { cost, requests, tokens };
}

export function useUsersInsightsData({
  account,
  range,
  selectedUsers,
  enabled = true,
}: UseUsersInsightsDataOpts) {
  // Always fetch all-time. The URL range slices everything client-side so
  // toggles repaint instantly without a network round-trip.
  const summaryQ = useAccountActivitySummary(account, undefined, undefined, { groupBy: "user", enabled });
  const usersQ = useUsersSummary(account, undefined, undefined, { enabled });
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

  const isAllSelected = selectedUsers.length > 0 && selectedUsers[0] === ALL_USERS_KEY;
  const noSelection = selectedUsers.length === 0;

  // Visible-user set for sparkline filtering. `null` means "no filter" (sum
  // across all users), matching how the chart treats no-selection / All.
  const sparklineFilter = useMemo<Set<string> | null>(() => {
    if (noSelection || isAllSelected) return null;
    return new Set(selectedUsers);
  }, [selectedUsers, noSelection, isAllSelected]);

  // Slice the all-time users data down to the URL window in a single pass —
  // produces both the per-user totals (Top Spenders / filter chips) and the
  // per-day sparkline arrays (StatCards) from one iteration over
  // `cost_over_time_by_user`.
  const { users: slicedUsers, sparklines: userSparklines } = useMemo(
    () => sliceUsersByRange(summary, usersData, range, sparklineFilter, memberIds),
    [summary, usersData, range, sparklineFilter, memberIds],
  );

  // Period the cards / chart should pretend the data covers. For all-time
  // we fall back to the server's period (often empty); for bounded ranges
  // we synthesize it from the URL window so daily_avg etc. compute correctly.
  const slicedPeriod = useMemo(() => {
    if (range === "all") return usersData?.period ?? { start: "", end: "", days: 0 };
    const { fromDate, toDate } = rangeWindow(range);
    const days = Math.max(1, Math.round((Date.parse(`${toDate}T00:00:00Z`) - Date.parse(`${fromDate}T00:00:00Z`)) / 86_400_000) + 1);
    return { start: `${fromDate}T00:00:00Z`, end: `${toDate}T23:59:59Z`, days };
  }, [range, usersData]);

  const allUserIds = useMemo(() => {
    const ids = new Set<string>();
    for (const u of slicedUsers) ids.add(classifyUserId(u.user_id, memberIds));
    // Filter the per-(day, user) rows to the current URL window — without
    // this, users active only outside the window would still appear in the
    // filter chips. All-time range yields empty bounds → the filter no-ops
    // and every row contributes (current behavior).
    const { fromDate, toDate } = rangeWindow(range);
    const bounded = !!fromDate && !!toDate;
    for (const row of summary?.cost_over_time_by_user ?? []) {
      if (bounded) {
        const d = row.date.slice(0, 10);
        if (d < fromDate || d > toDate) continue;
      }
      for (const u of row.users) ids.add(classifyUserId(u.user_id, memberIds));
    }
    return [...ids];
  }, [slicedUsers, summary, memberIds, range]);

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

  const filteredUsers = useMemo(() => {
    if (noSelection || isAllSelected) return slicedUsers;
    return slicedUsers.filter((u) => selectedUsers.includes(classifyUserId(u.user_id, memberIds)));
  }, [slicedUsers, selectedUsers, noSelection, isAllSelected, memberIds]);

  const chartVisibleUserIds = useMemo(() => {
    if (noSelection || isAllSelected) {
      const byKey = new Map<string, number>();
      for (const u of slicedUsers) {
        const key = classifyUserId(u.user_id, memberIds);
        byKey.set(key, (byKey.get(key) ?? 0) + u.cost_usd);
      }
      return [...byKey.entries()]
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5)
        .map(([k]) => k);
    }
    return selectedUsers;
  }, [slicedUsers, selectedUsers, noSelection, isAllSelected, memberIds]);

  const userCostOverTime = useMemo(
    () => buildUserCostOverTime(summary, range, chartVisibleUserIds, memberIds),
    [summary, range, chartVisibleUserIds, memberIds],
  );

  // Prior-period totals for the users-view StatCards. Same semantic as the
  // agents view: previous equal-length window, same user filter applied.
  const priorTotals = useMemo<ChangeTotals | null>(() => {
    if (range === "all") return null;
    const { fromDate, toDate } = rangeWindow(range);
    if (!fromDate || !toDate) return null;
    const prior = shiftPriorWindow(fromDate, toDate);
    if (!prior) return null;
    return sumUsersWindow(summary, prior.priorFrom, prior.priorTo, sparklineFilter, memberIds);
  }, [summary, range, sparklineFilter, memberIds]);

  const displaySummary = useMemo(
    () => recomputeTotalsFromUsers(filteredUsers, slicedPeriod, userSparklines, priorTotals),
    [filteredUsers, slicedPeriod, userSparklines, priorTotals],
  );

  const activeColorMap = useMemo(
    () => ({ ...allUserColorMap, [ALL_USERS_KEY]: "var(--color-indigo-500)" }),
    [allUserColorMap],
  );

  return {
    from: slicedPeriod.start,
    to: slicedPeriod.end,
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
    // Read off the SLICED users so a date range with no activity falls
    // through to the page-level EmptyState, matching the agents view.
    hasData: slicedUsers.some((u) => u.requests > 0),
  };
}
