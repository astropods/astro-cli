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
import { TopSpendersTable } from "@/components/activity/TopSpendersTable";
import { useInsightsData, useUsersInsightsData } from "@/components/activity/use-insights-data";
import { buildPeriodParams, type ActivityRange } from "@/components/activity/ranges";
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
  if (!ctx) return { account: null, summary: null, blueprintsData: null, usersData: null, members: null, view: "agents" as ActivityView, from: null, to: null };

  const url = new URL(request.url);
  const range = parseRange(url.searchParams.get("range"));
  const view = parseActivityView(url.searchParams.get("view"));
  const { from, to } = buildPeriodParams(range);
  const params: Record<string, string> = {};
  if (from) params.from = from;
  if (to) params.to = to;

  // Users view additionally requests group_by=user on the summary and fetches
  // the per-user breakdown. Agents view fetches the blueprints breakdown.
  // Members is fetched for the users view so the table's row classification
  // (member vs unauthorized) can render on first paint without a skeleton flash.
  const summaryParams = view === "users" ? { ...params, group_by: "user" } : params;

  const [summary, blueprintsData, usersData, members] = await Promise.all([
    ctx.api.getAccountObservabilitySummary(ctx.accountName, summaryParams).catch(() => null),
    view === "agents"
      ? ctx.api.getAccountBlueprintsSummary(ctx.accountName, params).catch(() => null)
      : Promise.resolve(null),
    view === "users"
      ? ctx.api.getAccountUsersSummary(ctx.accountName, params).catch(() => null)
      : Promise.resolve(null),
    view === "users"
      ? ctx.api.getAccountMembers(ctx.accountName, {}).catch(() => null)
      : Promise.resolve(null),
  ]);

  return {
    account: ctx.accountName,
    summary,
    blueprintsData,
    usersData,
    members,
    view,
    from: from ?? null,
    to: to ?? null,
  };
}

// Range / agent / user filter toggles change search params client-side, so
// they skip the loader (TanStack picks up the new key). Two cases DO re-run
// the loader:
//   1. Programmatic revalidation (currentUrl === nextUrl) — the org-switch
//      signal.
//   2. A view-param change — the loader fetches different data (blueprints
//      vs users summary), so cache priming for the new view needs fresh
//      server data instead of stale priming from the previous view.
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
  if (currentUrl.pathname === nextUrl.pathname) {
    return currentUrl.searchParams.get("view") !== nextUrl.searchParams.get("view");
  }
  return defaultShouldRevalidate;
}

export default function Insights({ loaderData }: Route.ComponentProps) {
  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (!ld?.account) return;
    // from/to are null for the "all time" range; the key factory accepts
    // undefined and the hook calls into the same key, so prime under both.
    const from = ld.from ?? undefined;
    const to = ld.to ?? undefined;
    const groupBy = ld.view === "users" ? "user" : undefined;
    if (ld.summary) {
      qc.setQueryData(observabilityKeys.activitySummary(ld.account, from, to, groupBy), ld.summary);
    }
    if (ld.blueprintsData) {
      qc.setQueryData(observabilityKeys.blueprintsSummary(ld.account, from, to), ld.blueprintsData);
    }
    if (ld.usersData) {
      qc.setQueryData(observabilityKeys.usersSummary(ld.account, from, to), ld.usersData);
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

          {view === "agents" ? (
            <AgentsTab
              account={activeAccount}
              range={range}
              selectedAgents={selectedAgents}
              onChangeAgents={setSelectedAgents}
              ssrFrom={loaderData?.from}
              ssrTo={loaderData?.to}
              viewToggle={viewToggle}
              timeRangeSelector={timeRangeSelector}
            />
          ) : (
            <UsersTab
              account={activeAccount}
              range={range}
              selectedUsers={selectedUsers}
              onChangeUsers={setSelectedUsers}
              ssrFrom={loaderData?.from}
              ssrTo={loaderData?.to}
              viewToggle={viewToggle}
              timeRangeSelector={timeRangeSelector}
            />
          )}
        </div>
      </div>
    </div>
  );
}

// ── Shared layout ───────────────────────────────────────────────────────────

function FilterRowGrid({ left, right, children }: { left: ReactNode; right: ReactNode; children: ReactNode }) {
  return (
    <div className="mb-6">
      <div className="grid grid-cols-1 @md:grid-cols-[auto_1fr_auto] items-center gap-3">
        {left}
        <div>{children}</div>
        <div className="flex items-center justify-end gap-3">{right}</div>
      </div>
    </div>
  );
}

interface InsightsBodyProps {
  range: ActivityRange;
  dateLabel: string;
  displaySummary: Parameters<typeof StatCards>[0]["data"];
  isLoading: boolean;
  hasData: boolean;
  empty: { title: string; description: string };
  chart: ReactNode;
  table: ReactNode;
}

function InsightsBody({ range, dateLabel, displaySummary, isLoading, hasData, empty, chart, table }: InsightsBodyProps) {
  return (
    <>
      {dateLabel && (
        <div className="mb-3 font-mono text-body-sm text-muted-foreground">{dateLabel}</div>
      )}

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
          <div className="mb-6 h-[300px]">{chart}</div>
          {table}
        </motion.div>
      )}
    </>
  );
}

// ── Agents tab ──────────────────────────────────────────────────────────────

interface AgentsTabProps {
  account: string;
  range: ActivityRange;
  selectedAgents: string[];
  onChangeAgents: (a: string[]) => void;
  ssrFrom: string | null | undefined;
  ssrTo: string | null | undefined;
  viewToggle: ReactNode;
  timeRangeSelector: ReactNode;
}

function AgentsTab({ account, range, selectedAgents, onChangeAgents, ssrFrom, ssrTo, viewToggle, timeRangeSelector }: AgentsTabProps) {
  const data = useInsightsData({ account, range, selectedAgents, ssrFrom, ssrTo });
  const days = RANGE_DAYS[range];
  return (
    <>
      <FilterRowGrid left={viewToggle} right={timeRangeSelector}>
        <AgentFilterBar
          value={selectedAgents}
          onValueChange={onChangeAgents}
          allAgentNames={data.allAgentNames}
          colorMap={data.allAgentColorMap}
        />
      </FilterRowGrid>
      <InsightsBody
        range={range}
        dateLabel={buildDateLabel(range, data.from, data.to)}
        displaySummary={data.displaySummary}
        isLoading={data.isLoading}
        hasData={data.hasData}
        empty={{
          title: "No insights for this period",
          description: "Deploy agents and start sending requests to see usage data here.",
        }}
        chart={
          <CostOverTimeChart
            data={data.agentCostOverTime}
            days={days}
            colorMap={data.activeColorMap}
            seriesLabels={{ __all__: "All Agents" }}
            variant={range === "all" ? "line" : "bar"}
          />
        }
        table={
          <TopSpendersTable
            mode="agents"
            blueprints={data.filteredBlueprints}
            loading={data.blueprintsLoading}
            groupLabel="Agent"
          />
        }
      />
    </>
  );
}

// ── Users tab ───────────────────────────────────────────────────────────────

interface UsersTabProps {
  account: string;
  range: ActivityRange;
  selectedUsers: string[];
  onChangeUsers: (u: string[]) => void;
  ssrFrom: string | null | undefined;
  ssrTo: string | null | undefined;
  viewToggle: ReactNode;
  timeRangeSelector: ReactNode;
}

function UsersTab({ account, range, selectedUsers, onChangeUsers, ssrFrom, ssrTo, viewToggle, timeRangeSelector }: UsersTabProps) {
  const data = useUsersInsightsData({ account, range, selectedUsers, ssrFrom, ssrTo });
  const days = RANGE_DAYS[range];
  return (
    <>
      <FilterRowGrid left={viewToggle} right={timeRangeSelector}>
        <UserFilterBar
          account={account}
          presentUserIds={data.allUserIds}
          value={selectedUsers}
          onValueChange={onChangeUsers}
          colorMap={data.allUserColorMap}
        />
      </FilterRowGrid>
      <InsightsBody
        range={range}
        dateLabel={buildDateLabel(range, data.from, data.to)}
        displaySummary={data.displaySummary}
        isLoading={data.isLoading}
        hasData={data.hasData}
        empty={{
          title: "No user activity for this period",
          description: "Once agents start receiving traced requests with a user_id, this view will populate.",
        }}
        chart={
          <CostOverTimeChart
            data={data.userCostOverTime}
            days={days}
            colorMap={data.activeColorMap}
            seriesLabels={data.userLabelMap}
            variant={range === "all" ? "line" : "bar"}
          />
        }
        table={
          <TopSpendersTable
            mode="users"
            account={account}
            users={data.filteredUsers}
            loading={data.usersLoading}
          />
        }
      />
    </>
  );
}
