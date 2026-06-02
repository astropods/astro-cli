import { type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { AnimatePresence, motion } from "motion/react";
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
import { useDeploymentsSummary, useUsersSummary } from "@/api/queries/observability";
import { useAccountMembers } from "@/api/queries/accounts";
import { useDeployments } from "@/api/queries/deployments";
import { useMemo } from "react";
import { isSlackUserId } from "@/components/activity/user-classification";
import { type ActivityRange, buildPeriodParams } from "@/components/activity/ranges";
import { formatDateShort } from "@/lib/format-utils";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { FilterInput } from "@/components/FilterInput";
import { Switch } from "@/components/ui/switch";
import { WarningPanel } from "@/components/ui/status-panel";
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
      account: null, summary: null, deploymentsSummary: null,
      usersData: null, members: null, deployments: null,
      includeArchived: false,
    };
  }

  // Read ?archived from the URL so a deep-link to ?archived=true primes
  // the right cache slot. Without this, a shared link lands cold on the
  // archived-included variant and the hook fires its own fetch on mount.
  const includeArchived = new URL(request.url).searchParams.get("archived") === "true";

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
  // no longer needed — those derive from deployments now.
  //
  // Deployments list (separate from the obs summary) primes the
  // AgentsUsedChips picker in the People view so the agent-name links
  // resolve to a Monitor tab on first paint instead of flashing through
  // the blueprint-detail fallback.
  const archivedParams: Record<string, string> = {};
  if (includeArchived) archivedParams.include_archived = "true";
  const summaryParams: Record<string, string> = { group_by: "user", ...archivedParams };
  const [summary, deploymentsSummary, usersData, members, deployments] = await Promise.all([
    ctx.api.getAccountObservabilitySummary(ctx.accountName, summaryParams).catch(() => null),
    ctx.api.getAccountDeploymentsSummary(ctx.accountName, archivedParams).catch(() => null),
    ctx.api.getAccountUsersSummary(ctx.accountName, {}).catch(() => null),
    ctx.api.getAccountMembers(ctx.accountName, {}).catch(() => null),
    ctx.api.listDeployments(ctx.accountName).catch(() => null),
  ]);

  return {
    account: ctx.accountName,
    summary,
    deploymentsSummary,
    usersData,
    members,
    deployments,
    includeArchived,
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
      qc.setQueryData(observabilityKeys.activitySummary(ld.account, undefined, undefined, "user", ld.includeArchived), ld.summary);
    }
    if (ld.deploymentsSummary) {
      qc.setQueryData(observabilityKeys.deploymentsSummary(ld.account, undefined, undefined, ld.includeArchived), ld.deploymentsSummary);
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
  const archived = searchParams.get("archived") === "true";

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

  function setArchived(next: boolean) {
    setSearchParams((prev) => {
      if (next) prev.set("archived", "true");
      else prev.delete("archived");
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
        archived={archived}
        onArchivedChange={setArchived}
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
  // True when the upstream metrics backend is unreachable; the page still
  // renders the zero-state KPIs and charts under a banner instead of erroring.
  metricsUnavailable?: boolean;
}

function InsightsBody({ range, displaySummary, chartLeft, chartRight, table, metricsUnavailable }: InsightsBodyProps) {
  return (
    <>
      {/* Banner deliberately surfaces upstream-metric unavailability instead
          of silently rendering zeros. Reasoning: zero-cost / zero-traces is
          a valid steady-state for accounts with little activity, so without
          the banner users can't tell "we have no data" from "we couldn't
          fetch your data right now." We keep the copy free of internal
          system names — users don't need to know about Langfuse to act on
          this (retry later, or contact support if persistent). */}
      {metricsUnavailable && (
        <div className="mb-4">
          <WarningPanel title="Metrics temporarily unavailable">
            We couldn&apos;t load up-to-date usage metrics. Try refreshing in a few minutes.
          </WarningPanel>
        </div>
      )}
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
  archived: boolean;
  onArchivedChange: (v: boolean) => void;
}

function InsightsView({
  account,
  range,
  view,
  onViewChange,
  query,
  onQueryChange,
  archived,
  onArchivedChange,
}: InsightsViewProps) {
  // ── Charts (view-independent) ────────────────────────────────────────────
  // Both useInsightsData and useDeploymentsSummary below pass the same
  // includeArchived flag so they share a single fetch via TanStack's
  // query-key dedupe.
  const chartsData = useInsightsData({ account, range, includeArchived: archived });
  const activeSpendSeries = useActiveSpendSeries(account, range, { includeArchived: archived });
  const days = RANGE_DAYS[range];

  // ── Agents-mode table data (all-time, per-deployment rows) ───────────────
  const deploymentsSummaryQ = useDeploymentsSummary(account, undefined, undefined, { includeArchived: archived });
  const allTimeDeployments = deploymentsSummaryQ.data?.deployments ?? [];

  // Deployments list (separate from the summary) feeds the AgentsUsedChips
  // picker on the People view — chips can reference the same agent_name
  // across multiple deployments, and clicking one should still route to a
  // specific Monitor tab.
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
  // Mirrors UsersTopSpenders' bucketing exactly: each named member and
  // each Slack user counts as one row; the unidentified and unattributed
  // buckets each count as one row only when non-empty. Collapsing all
  // Slack ids to a single sentinel would undercount the pill in
  // Slack-heavy workspaces — keep the per-id count explicit.
  const allUserBuckets = useMemo(() => {
    const namedIds = new Set<string>();
    const slackIds = new Set<string>();
    let hasUnidentified = false;
    let hasUnattributed = false;
    for (const u of allTimeUsers) {
      if (!u.user_id) hasUnattributed = true;
      else if (memberIds.has(u.user_id)) namedIds.add(u.user_id);
      else if (isSlackUserId(u.user_id)) slackIds.add(u.user_id);
      else hasUnidentified = true;
    }
    return namedIds.size + slackIds.size + (hasUnidentified ? 1 : 0) + (hasUnattributed ? 1 : 0);
  }, [allTimeUsers, memberIds]);

  // ── Search filter ─────────────────────────────────────────────────────────
  // Single free-text input filters whichever view is active. Match
  // deployments by agent_name / display_name / namespace; match users by
  // display_name / username / user_id. Empty query → no filtering.
  const needle = query.trim().toLowerCase();
  const filteredDeployments = useMemo(() => {
    if (!needle) return allTimeDeployments;
    return allTimeDeployments.filter((d) => {
      const haystack = `${d.agent_name} ${d.display_name ?? ""} ${d.namespace ?? ""}`.toLowerCase();
      return haystack.includes(needle);
    });
  }, [allTimeDeployments, needle]);
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
  const totalDeploymentCost = useMemo(
    () => allTimeDeployments.reduce((s, b) => s + b.cost_usd, 0),
    [allTimeDeployments],
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
      <div className="flex flex-wrap items-center gap-3">
        <ViewToggle
          value={view}
          onChange={onViewChange}
          usersCount={allUserBuckets || undefined}
          agentsCount={allTimeDeployments.length || undefined}
        />
        {view === "agents" && (
          <label className="flex cursor-pointer items-center gap-2 text-body-sm text-muted-foreground">
            <Switch checked={archived} onCheckedChange={onArchivedChange} aria-label="Show deleted deployments" />
            <span>Show deleted deployments</span>
          </label>
        )}
      </div>
      <FilterInput
        containerClassName="h-8 w-full @md:max-w-xs"
        placeholder="Search people or agents..."
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
      />
    </div>
  );

  const metricsUnavailable =
    chartsData.metricsUnavailable ||
    activeSpendSeries.metricsUnavailable ||
    deploymentsSummaryQ.data?.metrics_unavailable === true ||
    usersTableQ.data?.metrics_unavailable === true;

  return (
    <InsightsBody
        range={range}
        displaySummary={chartsData.displaySummary}
        metricsUnavailable={metricsUnavailable}
        chartLeft={
          <CostOverTimeChart
            data={chartsData.deploymentCostOverTime}
            days={days}
            colorMap={chartsData.colorMap}
            seriesLabels={chartsData.seriesLabels}
            variant={range === "all" ? "line" : "bar"}
          />
        }
        chartRight={<ActiveUsersSpendChart data={activeSpendSeries.data} days={days} />}
        table={
          // Crossfade the whole table when toggling People <-> Agents.
          // mode="wait" lets the old table fully exit before the new one
          // enters — keeps the layout stable through the transition and
          // avoids a brief frame where both tables (with different column
          // counts) try to occupy the same space. The ViewToggle inside
          // panelHeader animates with the rest; the toggle's own indicator
          // slide is independent and runs in parallel.
          <AnimatePresence mode="wait" initial={false}>
            <motion.div
              key={view}
              initial={{ opacity: 0, y: 4 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -4 }}
              transition={{ duration: 0.18, ease: "easeOut" }}
            >
              {view === "agents" ? (
                <TopSpendersTable
                  mode="agents"
                  deployments={filteredDeployments}
                  loading={deploymentsSummaryQ.isLoading}
                  groupLabel="Name"
                  account={account}
                  totalCost={totalDeploymentCost}
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
              )}
            </motion.div>
          </AnimatePresence>
        }
      />
  );
}
