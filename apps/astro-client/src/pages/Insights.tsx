import { type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { motion } from "motion/react";
import { ChartBarIcon } from "@heroicons/react/24/outline";
import { useActiveAccount } from "@/hooks/use-active-account";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { ViewToggle, parseActivityView, type ActivityView } from "@/components/activity/ViewToggle";
import { AgentFilterBar } from "@/components/activity/AgentFilterBar";
import { UserFilterBar } from "@/components/activity/UserFilterBar";
import { StatCards } from "@/components/activity/StatCards";
import { CostOverTimeChart } from "@/components/activity/CostOverTimeChart";
import { ActiveUsersSpendChart } from "@/components/activity/ActiveUsersSpendChart";
import { TopSpendersTable } from "@/components/activity/TopSpendersTable";
import {
  useInsightsData,
  useActiveSpendSeries,
  ALL_AGENTS_KEY,
} from "@/components/activity/use-insights-data";
import { useBlueprintsSummary, useUsersSummary } from "@/api/queries/observability";
import { useAccountMembers } from "@/api/queries/accounts";
import { useMemo } from "react";
import { classifyUserId, ALL_USERS_KEY } from "@/components/activity/user-classification";
import { buildModelColorMap } from "@/components/activity/model-colors";
import { type ActivityRange } from "@/components/activity/ranges";
import { formatDateShort } from "@/lib/format-utils";
import { dashboardPath } from "@/lib/routes";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageHeader } from "@/components/PageLayout";
import { EmptyState } from "@/components/EmptyState";
import { PageStarField } from "@/components/agent-detail/starfield/PageStarField";
import { getActiveAccount } from "@/lib/api.server";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { accountKeys, observabilityKeys } from "@/api/queries/keys";
import type { Route } from "./+types/Insights";

const RANGE_DAYS: Record<string, number> = { "7d": 7, "14d": 14, "30d": 30 };

function buildDateLabel(range: ActivityRange, from?: string, to?: string): string {
  if (range === "all") return "All time";
  if (!from || !to) return "";
  return `${formatDateShort(from)} – ${formatDateShort(to)}`;
}

function parseRange(raw: string | null): ActivityRange {
  return raw === "7d" || raw === "14d" || raw === "30d" || raw === "all" ? raw : "30d";
}

export async function loader({ request }: Route.LoaderArgs) {
  const ctx = await getActiveAccount(request);
  if (!ctx) return { account: null, summary: null, blueprintsData: null, usersData: null, members: null };

  // Both views share the same chart subtree (Agent Spend + Active Users +
  // Spend) and both view modes' tables mount independently, so InsightsView
  // unconditionally consumes all four data sources. Loader fetches all of
  // them up-front to warm the TanStack cache and avoid a post-hydration
  // skeleton flash when the user toggles the view. Range toggles never
  // re-run the loader — they slice this all-time data client-side. The
  // ?view= param is read client-side; the loader doesn't branch on it.
  //
  // Summary is fetched with group_by=user because that's the only summary
  // shape any consumer reads (powers the Active Users + Spend chart). The
  // model-grouped summary that used to feed StatCards/CostOverTimeChart is
  // no longer needed — those derive from blueprints now.
  const [summary, blueprintsData, usersData, members] = await Promise.all([
    ctx.api.getAccountObservabilitySummary(ctx.accountName, { group_by: "user" }).catch(() => null),
    ctx.api.getAccountBlueprintsSummary(ctx.accountName, {}).catch(() => null),
    ctx.api.getAccountUsersSummary(ctx.accountName, {}).catch(() => null),
    ctx.api.getAccountMembers(ctx.accountName, {}).catch(() => null),
  ]);

  return {
    account: ctx.accountName,
    summary,
    blueprintsData,
    usersData,
    members,
  };
}

// Range / agent / user / view filter toggles change search params client-side,
// so they skip the loader (TanStack picks up the new key). The only loader
// re-run is the programmatic revalidate signal used for org-switching
// (currentUrl === nextUrl). The view toggle no longer triggers a loader run —
// both views derive from the same client-side data sources now, so flipping
// `?view=` is a component-only update.
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
  });

  const { activeAccount } = useActiveAccount();
  const [searchParams, setSearchParams] = useSearchParams();
  const range = parseRange(searchParams.get("range"));
  const view = parseActivityView(searchParams.get("view"));

  const rawAgents = searchParams.get("agents");
  const selectedAgents = rawAgents ? rawAgents.split(",").filter(Boolean) : [];
  const rawUsers = searchParams.get("users");
  const selectedUsers = rawUsers ? rawUsers.split(",").filter(Boolean) : [];

  function setSelectedAgents(agents: string[]) {
    setSearchParams((prev) => {
      if (agents.length === 0) prev.delete("agents");
      else prev.set("agents", agents.join(","));
      return prev;
    }, { replace: true });
  }

  function setSelectedUsers(users: string[]) {
    setSearchParams((prev) => {
      if (users.length === 0) prev.delete("users");
      else prev.set("users", users.join(","));
      return prev;
    }, { replace: true });
  }

  function setView(next: ActivityView) {
    setSearchParams((prev) => {
      if (next === "agents") {
        prev.delete("view");
        prev.delete("users");
      } else {
        prev.set("view", next);
        prev.delete("agents");
      }
      return prev;
    }, { replace: true });
  }

  const viewToggle = <ViewToggle value={view} onChange={setView} />;
  const timeRangeSelector = (
    <TimeRangeSelector
      value={range}
      onChange={(r) => setSearchParams((prev) => { prev.set("range", r); return prev; }, { replace: true })}
    />
  );

  return (
    <div className="relative flex-1 overflow-hidden">
      <PageStarField className="absolute inset-0" />
      <div className="relative z-10 h-full overflow-y-auto">
        <div className="@container mx-auto w-full max-w-[1500px] px-6 pb-6 pt-6 md:px-8 md:pb-8 md:pt-8">
          <PageHeader
            title="Insights"
            description="Track usage, cost, and reliability across your agents."
            action={<PageScopeSwitcher />}
          />

          <InsightsView
            account={activeAccount}
            range={range}
            view={view}
            selectedAgents={selectedAgents}
            onChangeAgents={setSelectedAgents}
            selectedUsers={selectedUsers}
            onChangeUsers={setSelectedUsers}
            viewToggle={viewToggle}
            timeRangeSelector={timeRangeSelector}
          />
        </div>
      </div>
    </div>
  );
}

// ── Shared layout ───────────────────────────────────────────────────────────

function TopBar({ left, right }: { left: ReactNode; right: ReactNode }) {
  return (
    <div className="mb-6 flex flex-col gap-3 @md:flex-row @md:items-center @md:justify-between">
      {left}
      <div className="flex items-center gap-3">{right}</div>
    </div>
  );
}

interface InsightsBodyProps {
  range: ActivityRange;
  displaySummary: Parameters<typeof StatCards>[0]["data"];
  isLoading: boolean;
  hasData: boolean;
  empty: { title: string; description: string };
  // Two charts rendered side by side. Both react to the date-range chip;
  // neither reacts to the agent/user view toggle.
  chartLeft: ReactNode;
  chartRight: ReactNode;
  table: ReactNode;
  // Rendered immediately above the table — used for the view toggle +
  // filter bar so they sit next to the data they control.
  tableToolbar?: ReactNode;
}

function InsightsBody({ range, displaySummary, isLoading, hasData, empty, chartLeft, chartRight, table, tableToolbar }: InsightsBodyProps) {
  return (
    <>
      <motion.div initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }}>
        <StatCards data={displaySummary} showChange={range !== "all"} range={range} />
      </motion.div>

      {!isLoading && !hasData ? (
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
          <EmptyState
            variant="card"
            icon={<ChartBarIcon className="mx-auto size-10 text-faint-foreground" />}
            title={empty.title}
            description={empty.description}
            actions={[{ label: "Go to Agents", to: dashboardPath }]}
          />
        </motion.div>
      ) : (
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
          <div className="mb-6 grid grid-cols-1 gap-4 @xl:grid-cols-2">
            <div className="h-[300px]">{chartLeft}</div>
            <div className="h-[300px]">{chartRight}</div>
          </div>
          {tableToolbar && <div className="mb-3">{tableToolbar}</div>}
          {table}
        </motion.div>
      )}
    </>
  );
}

// ── Insights view ───────────────────────────────────────────────────────────

// InsightsView is the single component that stays mounted across view-toggle
// flips. Hooks for both views' table data are unconditionally evaluated; the
// query cache makes repeated fetches free, but more importantly the chart +
// stat-card subtree never unmounts when ?view= flips. Only the filter bar +
// table swap inside InsightsBody.
interface InsightsViewProps {
  account: string;
  range: ActivityRange;
  view: ActivityView;
  selectedAgents: string[];
  onChangeAgents: (a: string[]) => void;
  selectedUsers: string[];
  onChangeUsers: (u: string[]) => void;
  viewToggle: ReactNode;
  timeRangeSelector: ReactNode;
}

function InsightsView({
  account,
  range,
  view,
  selectedAgents,
  onChangeAgents,
  selectedUsers,
  onChangeUsers,
  viewToggle,
  timeRangeSelector,
}: InsightsViewProps) {
  // ── Charts (view-independent) ────────────────────────────────────────────
  const chartsData = useInsightsData({ account, range, selectedAgents: [] });
  const activeSpendSeries = useActiveSpendSeries(account, range);
  const days = RANGE_DAYS[range];
  const dateLabel = buildDateLabel(range, chartsData.from, chartsData.to);

  // ── Agents-mode table data (all-time) ────────────────────────────────────
  const agentsTableQ = useBlueprintsSummary(account, undefined, undefined);
  const allTimeBlueprints = agentsTableQ.data?.blueprints ?? [];
  const allAgentNamesForTable = useMemo(
    () => allTimeBlueprints.map((b) => b.agent_name),
    [allTimeBlueprints],
  );
  const allAgentColorMapForTable = useMemo(
    () => buildModelColorMap(allAgentNamesForTable),
    [allAgentNamesForTable],
  );
  const filteredBlueprintsForTable = useMemo(() => {
    if (selectedAgents.length > 0 && selectedAgents[0] !== ALL_AGENTS_KEY) {
      return allTimeBlueprints.filter((b) => selectedAgents.includes(b.agent_name));
    }
    return allTimeBlueprints;
  }, [allTimeBlueprints, selectedAgents]);

  // ── Users-mode table data (all-time) ─────────────────────────────────────
  const usersTableQ = useUsersSummary(account, undefined, undefined);
  const membersQ = useAccountMembers(account);
  const allTimeUsers = usersTableQ.data?.users ?? [];
  const memberIds = useMemo(
    () => new Set(membersQ.data?.members.map((m) => m.user_id) ?? []),
    [membersQ.data],
  );
  const allUserIds = useMemo(() => {
    const set = new Set<string>();
    for (const u of allTimeUsers) set.add(classifyUserId(u.user_id, memberIds));
    return [...set];
  }, [allTimeUsers, memberIds]);
  const allUserColorMap = useMemo(() => buildModelColorMap(allUserIds), [allUserIds]);
  const filteredUsers = useMemo(() => {
    if (selectedUsers.length === 0 || selectedUsers[0] === ALL_USERS_KEY) return allTimeUsers;
    return allTimeUsers.filter((u) => selectedUsers.includes(classifyUserId(u.user_id, memberIds)));
  }, [allTimeUsers, selectedUsers, memberIds]);

  return (
    <>
      <TopBar
        left={dateLabel ? <span className="font-mono text-body-sm text-muted-foreground">{dateLabel}</span> : null}
        right={timeRangeSelector}
      />
      <InsightsBody
        range={range}
        displaySummary={chartsData.displaySummary}
        isLoading={chartsData.isLoading}
        hasData={chartsData.hasData}
        empty={{
          title: "No insights for this period",
          description: "Deploy agents and start sending requests to see usage data here.",
        }}
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
        tableToolbar={
          <div className="flex flex-col gap-3 @md:flex-row @md:items-center">
            {viewToggle}
            <div className="flex-1">
              {view === "agents" ? (
                <AgentFilterBar
                  value={selectedAgents}
                  onValueChange={onChangeAgents}
                  allAgentNames={allAgentNamesForTable}
                  colorMap={allAgentColorMapForTable}
                />
              ) : (
                <UserFilterBar
                  account={account}
                  presentUserIds={allUserIds}
                  value={selectedUsers}
                  onValueChange={onChangeUsers}
                  colorMap={allUserColorMap}
                />
              )}
            </div>
          </div>
        }
        table={
          view === "agents" ? (
            <TopSpendersTable
              mode="agents"
              blueprints={filteredBlueprintsForTable}
              loading={agentsTableQ.isLoading}
              groupLabel="Agent"
              account={account}
              totalCount={allAgentNamesForTable.length}
            />
          ) : (
            <TopSpendersTable
              mode="users"
              account={account}
              users={filteredUsers}
              loading={usersTableQ.isLoading || membersQ.isLoading}
              totalCount={allUserIds.length}
            />
          )
        }
      />
    </>
  );
}
