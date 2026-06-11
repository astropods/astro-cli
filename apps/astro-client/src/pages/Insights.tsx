import { type ReactNode, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { motion } from "motion/react";
import { ArrowUpRight, Check, RefreshCw, TriangleAlert } from "lucide-react";
import { useActiveAccount } from "@/hooks/use-active-account";
import { cn } from "@/lib/utils";
import { PillToggleChrome } from "@/components/activity/PillToggle";
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
import { countSlackRowsMissingDetails, insightsUserIdentityKey } from "@/components/activity/insights-user-identity";
import { useSlackAccountConnect, useSlackAccountStatus } from "@/api/queries/slack";
import { isSlackUserId } from "@/components/activity/user-classification";
import { type ActivityRange, buildPeriodParams } from "@/components/activity/ranges";
import { formatDateShort } from "@/lib/format-utils";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { FilterInput } from "@/components/FilterInput";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { WarningPanel } from "@/components/ui/status-panel";
import { getActiveAccount } from "@/lib/api.server";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { accountKeys, deploymentKeys, observabilityKeys, slackKeys } from "@/api/queries/keys";
import type { Route } from "./+types/Insights";

const RANGE_DAYS: Record<string, number> = { "7d": 7, "14d": 14, "30d": 30, "90d": 90 };
const SLACK_OAUTH_PARAMS = ["slack_connected", "slack_user", "slack_team", "slack_error"] as const;
const SLACK_REFRESH_FEEDBACK_MS = 3_500;

type SlackRefreshStatus = "idle" | "refreshing" | "success" | "error";

function stripSlackOAuthParams(params: URLSearchParams) {
  for (const key of SLACK_OAUTH_PARAMS) params.delete(key);
  return params;
}

function buildDateLabel(range: ActivityRange): string {
  const { from, to } = buildPeriodParams(range);
  return `${formatDateShort(from)} – ${formatDateShort(to)}`;
}

function parseRange(raw: string | null): ActivityRange {
  // Stale "?range=all" deep-links from before the all-time range was retired
  // fall through to the 30d default rather than 404ing the page.
  return raw === "7d" || raw === "14d" || raw === "30d" || raw === "90d" ? raw : "30d";
}

export function insightsSlackResyncQueryKeys(account: string) {
  return [
    observabilityKeys.activitySummary(account, undefined, undefined, "user"),
    observabilityKeys.deploymentsSummary(account, undefined, undefined),
    observabilityKeys.usersSummary(account, undefined, undefined),
    accountKeys.members(account),
    slackKeys.accountStatus(account),
  ];
}

async function invalidateInsightsSlackResyncQueries(queryClient: QueryClient, account: string) {
  const queryKeys = insightsSlackResyncQueryKeys(account);
  await Promise.all(
    queryKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey, refetchType: "none" })),
  );
  await Promise.all(
    queryKeys.map((queryKey) => queryClient.refetchQueries(
      { queryKey, type: "active" },
      { throwOnError: true },
    )),
  );
}

export async function loader({ request }: Route.LoaderArgs) {
  const ctx = await getActiveAccount(request);
  if (!ctx) {
    return {
      account: null, summary: null, deploymentsSummary: null,
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
  // no longer needed — those derive from deployments now.
  //
  // Deployments list (separate from the obs summary) primes the
  // AgentsUsedChips picker in the People view so the agent-name links
  // resolve to a Monitor tab on first paint instead of flashing through
  // the blueprint-detail fallback.
  const [summary, deploymentsSummary, usersData, members, deployments] = await Promise.all([
    ctx.api.getAccountObservabilitySummary(ctx.accountName, { group_by: "user" }).catch(() => null),
    ctx.api.getAccountDeploymentsSummary(ctx.accountName, {}).catch(() => null),
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
  const queryClient = useQueryClient();
  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (!ld?.account) return;
    // Loader-fetched data primed under the same keys the hooks read on
    // mount. Summary is always group_by=user (matches useActiveSpendSeries).
    if (ld.summary) {
      qc.setQueryData(observabilityKeys.activitySummary(ld.account, undefined, undefined, "user"), ld.summary);
    }
    if (ld.deploymentsSummary) {
      qc.setQueryData(observabilityKeys.deploymentsSummary(ld.account, undefined, undefined), ld.deploymentsSummary);
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
  const slackConnected = searchParams.get("slack_connected") === "true";
  const slackError = searchParams.get("slack_error");
  const hasSlackOAuthParam = SLACK_OAUTH_PARAMS.some((key) => searchParams.has(key));
  const accountForSlackResync = activeAccount || loaderData.account || "";
  const [slackRefreshStatus, setSlackRefreshStatus] = useState<SlackRefreshStatus>("idle");

  useEffect(() => {
    if (!hasSlackOAuthParam) return;
    if (slackConnected) {
      if (!accountForSlackResync) {
        setSearchParams(stripSlackOAuthParams, { replace: true });
        return;
      }
      setSlackRefreshStatus("refreshing");
      let active = true;
      void invalidateInsightsSlackResyncQueries(queryClient, accountForSlackResync)
        .then(() => {
          if (active) setSlackRefreshStatus("success");
        })
        .catch(() => {
          if (active) setSlackRefreshStatus("error");
        })
        .finally(() => {
          if (!active) return;
          setSearchParams(stripSlackOAuthParams, { replace: true });
        });
      return () => {
        active = false;
      };
    }
    if (slackError) {
      setSlackRefreshStatus("error");
    }
    setSearchParams(stripSlackOAuthParams, { replace: true });
  }, [accountForSlackResync, hasSlackOAuthParam, queryClient, setSearchParams, slackConnected, slackError]);

  useEffect(() => {
    if (slackRefreshStatus !== "success" && slackRefreshStatus !== "error") return;
    const timeout = window.setTimeout(() => setSlackRefreshStatus("idle"), SLACK_REFRESH_FEEDBACK_MS);
    return () => window.clearTimeout(timeout);
  }, [slackRefreshStatus]);

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
        slackRefreshStatus={slackRefreshStatus}
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
        <StatCards data={displaySummary} showChange range={range} />
      </motion.div>
      <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
        <div className="mb-6 grid grid-cols-1 gap-4 @xl:grid-cols-2">
          <div className="h-[300px]">{chartLeft}</div>
          <div className="h-[300px]">{chartRight}</div>
        </div>
        {table}
      </motion.div>
      {/* Insights data is served from a 6-hourly refresh — the server's
          cache holds last-known-good metrics so a Langfuse outage
          doesn't blank the page. Keep the note muted; it's a
          disclaimer, not a status. */}
      <p className="mt-6 text-center text-body-sm text-faint-foreground">
        Updated results may take up to 6 hours to reflect on this page.
      </p>
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
  slackRefreshStatus: SlackRefreshStatus;
}

function InsightsView({
  account,
  range,
  view,
  onViewChange,
  query,
  onQueryChange,
  slackRefreshStatus,
}: InsightsViewProps) {
  // ── Charts (view-independent) ────────────────────────────────────────────
  const chartsData = useInsightsData({ account, range });
  const activeSpendSeries = useActiveSpendSeries(account, range);
  const days = RANGE_DAYS[range];

  // ── Agents-mode table data (all-time, per-deployment rows) ───────────────
  const deploymentsSummaryQ = useDeploymentsSummary(account, undefined, undefined);
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
  const slackRowsMissingDetails = useMemo(
    () => countSlackRowsMissingDetails(allTimeUsers),
    [allTimeUsers],
  );
  const showSlackDetailsAction = slackRowsMissingDetails > 0 && !usersTableQ.isLoading;
  const slackStatusQ = useSlackAccountStatus(account, {
    enabled: showSlackDetailsAction,
  });
  const slackConnect = useSlackAccountConnect(account);
  const slackConnected = (slackStatusQ.data?.workspaces.length ?? 0) > 0;
  const members = membersQ.data?.members ?? [];
  const memberIds = useMemo(
    () => new Set(members.map((m) => m.user_id)),
    [members],
  );
  // Mirrors TopSpendersTable's row count: each named member, each Slack
  // user, and each unidentified user_id renders as its own row; the
  // unattributed bucket collapses to a single row when non-empty.
  const allUserBuckets = useMemo(() => {
    const namedIds = new Set<string>();
    const slackIds = new Set<string>();
    const unidentifiedIds = new Set<string>();
    let hasUnattributed = false;
    for (const u of allTimeUsers) {
      if (!u.user_id) hasUnattributed = true;
      else if (memberIds.has(u.user_id)) namedIds.add(u.user_id);
      else if (isSlackUserId(u.user_id)) slackIds.add(insightsUserIdentityKey(u));
      else unidentifiedIds.add(u.user_id);
    }
    return namedIds.size + slackIds.size + unidentifiedIds.size + (hasUnattributed ? 1 : 0);
  }, [allTimeUsers, memberIds]);

  // ── Search filter ─────────────────────────────────────────────────────────
  // Single free-text input filters whichever view is active before the table
  // applies its visible-row window, so search can match rows hidden behind
  // Show more. Match deployments by agent_name / display_name / namespace;
  // match users by display_name / username / user_id. Empty query → no filtering.
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
      const d = u.user_details;
      const haystack = [
        m?.display_name,
        m?.username,
        u.user_id,
        d?.display_name,
        d?.username,
      ].filter(Boolean).join(" ").toLowerCase();
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

  const handleSlackDetails = () => {
    const returnPath = `${window.location.pathname || "/insights"}${window.location.search}`;
    slackConnect.mutate(returnPath, {
      onSuccess: (data) => {
        if (data.redirect_url) window.location.href = data.redirect_url;
      },
    });
  };
  const showSlackRefreshFeedback = slackRefreshStatus !== "idle";
  const showSlackDetailsButton = showSlackDetailsAction || showSlackRefreshFeedback;
  const slackActionLabel = slackConnect.isPending
    ? "Opening Slack..."
    : slackRefreshStatus === "refreshing"
      ? "Refreshing..."
      : slackRefreshStatus === "success"
        ? "Updated"
        : slackRefreshStatus === "error"
          ? "Refresh failed"
          : slackConnected
            ? "Resync Slack"
            : "Connect Slack";
  const slackActionButton = (
    <button
      type="button"
      className={cn(
        "relative inline-flex items-center gap-1.5 rounded-[5px] px-3 py-1 text-body-sm text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-50",
        showSlackRefreshFeedback && "pointer-events-none",
        slackRefreshStatus === "success" && "text-success hover:text-success",
        slackRefreshStatus === "error" && "text-destructive hover:text-destructive",
      )}
      aria-label={slackConnected ? "Resync Slack" : "Connect Slack"}
      aria-live="polite"
      disabled={slackConnect.isPending || slackRefreshStatus === "refreshing"}
      onClick={handleSlackDetails}
    >
      {slackRefreshStatus === "success" ? (
        <Check className="size-3.5 shrink-0" />
      ) : slackRefreshStatus === "error" ? (
        <TriangleAlert className="size-3.5 shrink-0" />
      ) : (slackConnect.isPending || slackConnected || slackRefreshStatus === "refreshing") && (
        <RefreshCw
          className={`size-3.5 shrink-0 ${slackConnect.isPending || slackRefreshStatus === "refreshing" ? "animate-spin" : ""}`}
        />
      )}
      {slackActionLabel}
      {!slackConnect.isPending && !slackConnected && !showSlackRefreshFeedback && (
        <ArrowUpRight className="size-3.5 shrink-0" aria-hidden />
      )}
    </button>
  );
  const shouldShowConnectTooltip = !slackConnected && !slackConnect.isPending && !showSlackRefreshFeedback;

  // Counts shown in the toggle pills are the un-filtered totals — the pill
  // reflects how much data exists, not how much the current search returns.
  // Toggle + search render inside the table's bordered container via the
  // Table primitive's `header` slot.
  const panelHeader = (
    <div className="flex flex-col gap-3 @md:flex-row @md:items-center @md:justify-between">
      <div className="flex items-center gap-3">
        <span className="font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">
          View by
        </span>
        <ViewToggle
          value={view}
          onChange={onViewChange}
          usersCount={allUserBuckets || undefined}
          agentsCount={allTimeDeployments.length || undefined}
        />
      </div>
      <div className="flex flex-col gap-2 @md:flex-row @md:items-center">
        {showSlackDetailsButton && (
          <PillToggleChrome size="md" inline className="shrink-0">
            {shouldShowConnectTooltip ? (
              <TooltipProvider delayDuration={200}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    {slackActionButton}
                  </TooltipTrigger>
                  <TooltipContent side="top">
                    Connect to identify Slack users.
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            ) : slackActionButton}
          </PillToggleChrome>
        )}
        <FilterInput
          containerClassName="h-8 w-full shrink-0 @md:w-80"
          placeholder="Search by name"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
        />
      </div>
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
            variant={days > 60 ? "line" : "bar"}
          />
        }
        chartRight={<ActiveUsersSpendChart data={activeSpendSeries.data} days={days} />}
        table={
          view === "agents" ? (
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
          )
        }
      />
  );
}
