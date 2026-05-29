import { type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { motion } from "motion/react";
import { useActiveAccount } from "@/hooks/use-active-account";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { ViewToggle, parseActivityView, type ActivityView } from "@/components/activity/ViewToggle";
import { StatCards } from "@/components/activity/StatCards";
import { CostOverTimeChart } from "@/components/activity/CostOverTimeChart";
import { ActiveUsersSpendChart } from "@/components/activity/ActiveUsersSpendChart";
import { TopSpendersTable } from "@/components/activity/TopSpendersTable";
import {
  useInsightsData,
  useActiveSpendSeries,
} from "@/components/activity/use-insights-data";
import { useBlueprintsSummary, useUsersSummary } from "@/api/queries/observability";
import { useAccountMembers } from "@/api/queries/accounts";
import { useDeployments } from "@/api/queries/deployments";
import { useMemo } from "react";
import { classifyUserId } from "@/components/activity/user-classification";
import { type ActivityRange, buildPeriodParams } from "@/components/activity/ranges";
import { formatDateShort } from "@/lib/format-utils";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { FilterInput } from "@/components/FilterInput";
import { getActiveAccount } from "@/lib/api.server";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { accountKeys, deploymentKeys, observabilityKeys } from "@/api/queries/keys";
import type { Route } from "./+types/Insights";

const RANGE_DAYS: Record<string, number> = { "7d": 7, "14d": 14, "30d": 30 };

function buildDateLabel(range: ActivityRange): string {
  if (range === "all") return "All time";
  const { from, to } = buildPeriodParams(range);
  if (!from || !to) return "";
  return `${formatDateShort(from)} – ${formatDateShort(to)}`;
}

function parseRange(raw: string | null): ActivityRange {
  return raw === "7d" || raw === "14d" || raw === "30d" || raw === "all" ? raw : "30d";
}

export async function loader({ request }: Route.LoaderArgs) {
  const ctx = await getActiveAccount(request);
  if (!ctx) {
    return {
      account: null, summary: null, blueprintsData: null,
      usersData: null, members: null, deployments: null,
    };
  }

  // Both views share the same chart subtree (Agent Spend + Active Users +
  // Spend) and both view modes' tables mount independently, so InsightsView
  // unconditionally consumes all five data sources. Loader fetches all of
  // them up-front to warm the TanStack cache and avoid a post-hydration
  // skeleton flash when the user toggles the view. Range toggles never
  // re-run the loader — they slice this all-time data client-side. The
  // ?view= param is read client-side; the loader doesn't branch on it.
  //
  // Summary is fetched with group_by=user because that's the only summary
  // shape any consumer reads (powers the Active Users + Spend chart). The
  // model-grouped summary that used to feed StatCards/CostOverTimeChart is
  // no longer needed — those derive from blueprints now.
  //
  // Deployments are primed so the agent-name picker has its 1-to-many
  // agent_name → deployments map ready on first paint (otherwise the row
  // links flash through "no deployments → blueprint detail" before the
  // useDeployments fetch resolves).
  const [summary, blueprintsData, usersData, members, deployments] = await Promise.all([
    ctx.api.getAccountObservabilitySummary(ctx.accountName, { group_by: "user" }).catch(() => null),
    ctx.api.getAccountBlueprintsSummary(ctx.accountName, {}).catch(() => null),
    ctx.api.getAccountUsersSummary(ctx.accountName, {}).catch(() => null),
    ctx.api.getAccountMembers(ctx.accountName, {}).catch(() => null),
    ctx.api.listDeployments(ctx.accountName).catch(() => null),
  ]);

  return {
    account: ctx.accountName,
    summary,
    blueprintsData,
    usersData,
    members,
    deployments,
  };
}

// Range / view / search-query toggles change search params client-side, so
// they skip the loader (TanStack picks up the new key for any per-range
// fetches; view + q are pure client-side filters). The only loader re-run
// is the programmatic revalidate signal used for org-switching
// (currentUrl === nextUrl).
export function shouldRevalidate({
  currentUrl,
  nextUrl,
  defaultShouldRevalidate,
}: {
  currentUrl: URL;
  nextUrl: URL;
  defaultShouldRevalidate: boolean;
}) {
  if (currentUrl.toString() === nextUrl.toString()) return true;
  if (currentUrl.pathname === nextUrl.pathname) return false;
  return defaultShouldRevalidate;
}

export default function Insights({ loaderData }: Route.ComponentProps) {
  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (!ld?.account) return;
    // Loader-fetched data primed under the same keys the hooks read on
    // mount. Summary is always group_by=user (matches useActiveSpendSeries).
    if (ld.summary) {
      qc.setQueryData(observabilityKeys.activitySummary(ld.account, undefined, undefined, "user"), ld.summary);
    }
    if (ld.blueprintsData) {
      qc.setQueryData(observabilityKeys.blueprintsSummary(ld.account, undefined, undefined), ld.blueprintsData);
    }
    if (ld.usersData) {
      qc.setQueryData(observabilityKeys.usersSummary(ld.account, undefined, undefined), ld.usersData);
    }
    if (ld.members) {
      // Without this, the users tab flashes a skeleton on hard refresh because
      // useAccountMembers fires post-hydration and gates classification.
      qc.setQueryData(accountKeys.members(ld.account), ld.members);
    }
    if (ld.deployments) {
      // Same rationale as `members`: the agent-name picker's 1-to-many map
      // is built from `useDeployments`, so without priming the row links
      // flash through their fallback (blueprint detail) before the deploy
      // fetch resolves.
      qc.setQueryData(deploymentKeys.all(ld.account), ld.deployments);
    }
  });

  const { activeAccount } = useActiveAccount();
  const [searchParams, setSearchParams] = useSearchParams();
  const range = parseRange(searchParams.get("range"));
  const view = parseActivityView(searchParams.get("view"));
  const q = searchParams.get("q") ?? "";

  function setView(next: ActivityView) {
    // Toggling views always clears the current search — a People-view term
    // would otherwise empty the Agents table (and vice versa).
    setSearchParams((prev) => {
      if (next === "agents") prev.delete("view");
      else prev.set("view", next);
      prev.delete("q");
      return prev;
    }, { replace: true });
  }

  function setQuery(next: string) {
    setSearchParams((prev) => {
      if (next === "") prev.delete("q");
      else prev.set("q", next);
      return prev;
    }, { replace: true });
  }

  const dateLabel = buildDateLabel(range);

  // Header right side packs three controls onto one row: date label (left),
  // range chips (middle), scope switcher (right). The date label tracks the
  // range chip so the user sees the resolved window without scanning back to
  // a separate sub-bar.
  const headerAction = (
    <div className="flex items-center gap-3">
      {dateLabel && (
        <span className="hidden text-body-sm text-muted-foreground @md:inline">
          {dateLabel}
        </span>
      )}
      <TimeRangeSelector
        value={range}
        onChange={(r) => setSearchParams((prev) => { prev.set("range", r); return prev; }, { replace: true })}
      />
      <PageScopeSwitcher />
    </div>
  );

  return (
    <PageContainer outerClassName="bg-background">
      <PageHeader
        title="Insights"
        description="Track usage, cost, and reliability across your organization."
        action={headerAction}
      />

      <InsightsView
        account={activeAccount}
        range={range}
        view={view}
        onViewChange={setView}
        query={q}
        onQueryChange={setQuery}
      />
    </PageContainer>
  );
}

// ── Shared layout ───────────────────────────────────────────────────────────

interface InsightsBodyProps {
  range: ActivityRange;
  displaySummary: Parameters<typeof StatCards>[0]["data"];
  // Two charts rendered side by side. Both react to the date-range chip;
  // neither reacts to the agent/user view toggle. When there's no data,
  // each chart renders its own in-card "No spend yet" placeholder
  // (mirrors the Monitor tab pattern on the agent detail page) — keeping
  // the page chrome shows the user what data will appear here.
  chartLeft: ReactNode;
  chartRight: ReactNode;
  // Toggle + search live inside the table's bordered container via the
  // Table primitive's `header` slot — see TopSpendersTable's panelHeader.
  table: ReactNode;
}

function InsightsBody({ range, displaySummary, chartLeft, chartRight, table }: InsightsBodyProps) {
  return (
    <>
      <motion.div initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }}>
        <StatCards data={displaySummary} showChange={range !== "all"} range={range} />
      </motion.div>
      <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
        <div className="mb-6 grid grid-cols-1 gap-4 @xl:grid-cols-2">
          <div className="h-[300px]">{chartLeft}</div>
          <div className="h-[300px]">{chartRight}</div>
        </div>
        {table}
      </motion.div>
    </>
  );
}

// ── Insights view ───────────────────────────────────────────────────────────

// InsightsView is the single component that stays mounted across view-toggle
// flips. Hooks for both views' table data are unconditionally evaluated; the
// query cache makes repeated fetches free, but more importantly the chart +
// stat-card subtree never unmounts when ?view= flips. Only the table body
// swaps when toggling People <-> Agents.
interface InsightsViewProps {
  account: string;
  range: ActivityRange;
  view: ActivityView;
  onViewChange: (v: ActivityView) => void;
  query: string;
  onQueryChange: (q: string) => void;
}

function InsightsView({
  account,
  range,
  view,
  onViewChange,
  query,
  onQueryChange,
}: InsightsViewProps) {
  // ── Charts (view-independent) ────────────────────────────────────────────
  const chartsData = useInsightsData({ account, range, selectedAgents: [] });
  const activeSpendSeries = useActiveSpendSeries(account, range);
  const days = RANGE_DAYS[range];

  // ── Agents-mode table data (all-time) ────────────────────────────────────
  const agentsTableQ = useBlueprintsSummary(account, undefined, undefined);
  const allTimeBlueprints = agentsTableQ.data?.blueprints ?? [];
  // Map agent_name → all deployments with that name. Blueprints data rolls
  // up multi-region deployments under one agent_name, but the table row's
  // link needs a specific deployment to target. The picker (in
  // TopSpendersTable) handles 0 / 1 / 2+ cases so the 1-to-many shape stays
  // visible to the user without splitting the row.
  const deploymentsQ = useDeployments(account);
  const deploymentsByAgent = useMemo(() => {
    const m = new Map<string, Array<{ id: string; name: string; display_name?: string; namespace?: string }>>();
    for (const d of deploymentsQ.data?.deployments ?? []) {
      const list = m.get(d.name);
      const ref = { id: d.id, name: d.name, display_name: d.display_name, namespace: d.namespace };
      if (list) list.push(ref);
      else m.set(d.name, [ref]);
    }
    return m;
  }, [deploymentsQ.data]);

  // ── Users-mode table data (all-time) ─────────────────────────────────────
  const usersTableQ = useUsersSummary(account, undefined, undefined);
  const membersQ = useAccountMembers(account);
  const allTimeUsers = usersTableQ.data?.users ?? [];
  const members = membersQ.data?.members ?? [];
  const memberIds = useMemo(
    () => new Set(members.map((m) => m.user_id)),
    [members],
  );
  const allUserBuckets = useMemo(() => {
    const set = new Set<string>();
    for (const u of allTimeUsers) set.add(classifyUserId(u.user_id, memberIds));
    return set.size;
  }, [allTimeUsers, memberIds]);

  // ── Search filter ─────────────────────────────────────────────────────────
  // Single free-text input filters whichever view is active. Match agents by
  // agent_name; match users by display_name / username / user_id (whichever
  // is available — falls back through). Empty query → no filtering.
  const needle = query.trim().toLowerCase();
  const filteredBlueprints = useMemo(() => {
    if (!needle) return allTimeBlueprints;
    return allTimeBlueprints.filter((b) => b.agent_name.toLowerCase().includes(needle));
  }, [allTimeBlueprints, needle]);
  const memberById = useMemo(
    () => new Map(members.map((m) => [m.user_id, m])),
    [members],
  );
  const filteredUsers = useMemo(() => {
    if (!needle) return allTimeUsers;
    return allTimeUsers.filter((u) => {
      const m = memberById.get(u.user_id);
      const haystack = `${m?.display_name ?? ""} ${m?.username ?? ""} ${u.user_id}`.toLowerCase();
      return haystack.includes(needle);
    });
  }, [allTimeUsers, memberById, needle]);

  // % Total denominator: total cost across the un-filtered population, so
  // percentages stay anchored while the user searches. Computed once per
  // dataset change rather than on every keystroke.
  const totalAgentCost = useMemo(
    () => allTimeBlueprints.reduce((s, b) => s + b.cost_usd, 0),
    [allTimeBlueprints],
  );
  const totalUserCost = useMemo(
    () => allTimeUsers.reduce((s, u) => s + u.cost_usd, 0),
    [allTimeUsers],
  );

  // Counts shown in the toggle pills are the un-filtered totals — the pill
  // reflects how much data exists, not how much the current search returns.
  // Toggle + search render inside the table's bordered container via the
  // Table primitive's `header` slot.
  const panelHeader = (
    <div className="flex flex-col gap-3 @md:flex-row @md:items-center @md:justify-between">
      <ViewToggle
        value={view}
        onChange={onViewChange}
        usersCount={allUserBuckets || undefined}
        agentsCount={allTimeBlueprints.length || undefined}
      />
      <FilterInput
        containerClassName="h-8 w-full @md:max-w-xs"
        placeholder="Search people or agents..."
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
      />
    </div>
  );

  return (
    <InsightsBody
        range={range}
        displaySummary={chartsData.displaySummary}
        chartLeft={
          <CostOverTimeChart
            data={chartsData.agentCostOverTime}
            days={days}
            colorMap={chartsData.activeColorMap}
            seriesLabels={{ __all__: "All Agents" }}
            variant={range === "all" ? "line" : "bar"}
          />
        }
        chartRight={<ActiveUsersSpendChart data={activeSpendSeries.data} days={days} />}
        table={
          view === "agents" ? (
            <TopSpendersTable
              mode="agents"
              blueprints={filteredBlueprints}
              loading={agentsTableQ.isLoading}
              groupLabel="Name"
              account={account}
              deploymentsByAgent={deploymentsByAgent}
              totalCost={totalAgentCost}
              panelHeader={panelHeader}
            />
          ) : (
            <TopSpendersTable
              mode="users"
              account={account}
              users={filteredUsers}
              loading={usersTableQ.isLoading || membersQ.isLoading}
              deploymentsByAgent={deploymentsByAgent}
              totalCost={totalUserCost}
              panelHeader={panelHeader}
            />
          )
        }
      />
  );
}
