import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react";
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
import {
  TopSpendersTable,
  type AgentSortKey,
  type TopSpendersSortDirection,
  type UserSortKey,
} from "@/components/activity/TopSpendersTable";
import { useAccountInsights } from "@/api/queries/observability";
import { useSlackAccountConnect, useSlackAccountStatus } from "@/api/queries/slack";
import { type ActivityRange, buildPeriodParams } from "@/components/activity/ranges";
import { formatDateShort } from "@/lib/format-utils";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { FilterInput } from "@/components/FilterInput";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { WarningPanel } from "@/components/ui/status-panel";
import { getActiveAccount } from "@/lib/api.server";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { observabilityKeys, slackKeys } from "@/api/queries/keys";
import type { InsightsQueryParams, InsightsResponse } from "@/lib/api";
import type { Route } from "./+types/Insights";

const RANGE_DAYS: Record<string, number> = { "7d": 7, "14d": 14, "30d": 30, "90d": 90 };
const SLACK_OAUTH_PARAMS = ["slack_connected", "slack_user", "slack_team", "slack_error"] as const;
const SLACK_REFRESH_FEEDBACK_MS = 3_500;
const SEARCH_DEBOUNCE_MS = 300;
const DEFAULT_TABLE_LIMIT = 5;
const TABLE_PAGE_SIZE = 10;
const DEFAULT_AGENT_SORT: AgentSortKey = "cost_usd";
const DEFAULT_USER_SORT: UserSortKey = "cost_usd";

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
    observabilityKeys.insights(account),
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

function buildInsightsQueryParams({
  query = "",
  agentsLimit = DEFAULT_TABLE_LIMIT,
  peopleLimit = DEFAULT_TABLE_LIMIT,
  agentSortKey = DEFAULT_AGENT_SORT,
  agentSortDirection = "desc",
  peopleSortKey = DEFAULT_USER_SORT,
  peopleSortDirection = "desc",
  skipRanges = false,
}: {
  query?: string;
  agentsLimit?: number;
  peopleLimit?: number;
  agentSortKey?: AgentSortKey;
  agentSortDirection?: TopSpendersSortDirection;
  peopleSortKey?: UserSortKey;
  peopleSortDirection?: TopSpendersSortDirection;
  skipRanges?: boolean;
}): InsightsQueryParams {
  const trimmedQuery = query.trim();
  return {
    q: trimmedQuery || undefined,
    agents_limit: String(agentsLimit),
    agents_offset: "0",
    agents_sort: agentSortKey,
    agents_direction: agentSortDirection,
    people_limit: String(peopleLimit),
    people_offset: "0",
    people_sort: peopleSortKey,
    people_direction: peopleSortDirection,
    skip_ranges: skipRanges ? "true" : undefined,
  };
}

function insightsTableParamsSignature(params: InsightsQueryParams): string {
  return [
    params.q ?? "",
    params.agents_limit ?? "",
    params.agents_offset ?? "",
    params.agents_sort ?? "",
    params.agents_direction ?? "",
    params.people_limit ?? "",
    params.people_offset ?? "",
    params.people_sort ?? "",
    params.people_direction ?? "",
  ].join("\u0000");
}

export async function loader({ request }: Route.LoaderArgs) {
  const ctx = await getActiveAccount(request);
  if (!ctx) {
    return { account: null, insights: null, insightsParams: buildInsightsQueryParams({}) };
  }

  const url = new URL(request.url);
  const insightsParams = buildInsightsQueryParams({ query: url.searchParams.get("q") ?? "" });
  const insights = await ctx.api.getAccountInsights(ctx.accountName, insightsParams).catch(() => null);
  return { account: ctx.accountName, insights, insightsParams };
}

// Range / view / search-query toggles change search params client-side, so
// they skip the loader. The consolidated Insights response contains every
// supported range; view + q are pure client-side display choices. The only
// loader re-run is the programmatic revalidate signal used for org-switching
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
    if (ld.insights) {
      qc.setQueryData(observabilityKeys.insights(ld.account, ld.insightsParams), ld.insights);
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

  const setView = useCallback((next: ActivityView) => {
    // Toggling views always clears the current search — a People-view term
    // would otherwise empty the Agents table (and vice versa).
    setSearchParams((prev) => {
      if (next === "agents") prev.delete("view");
      else prev.set("view", next);
      prev.delete("q");
      return prev;
    }, { replace: true });
  }, [setSearchParams]);

  const setQuery = useCallback((next: string) => {
    setSearchParams((prev) => {
      if (next === "") prev.delete("q");
      else prev.set("q", next);
      return prev;
    }, { replace: true });
  }, [setSearchParams]);

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
  const [agentsLimit, setAgentsLimit] = useState(DEFAULT_TABLE_LIMIT);
  const [peopleLimit, setPeopleLimit] = useState(DEFAULT_TABLE_LIMIT);
  const [agentSortKey, setAgentSortKey] = useState<AgentSortKey>(DEFAULT_AGENT_SORT);
  const [agentSortDirection, setAgentSortDirection] = useState<TopSpendersSortDirection>("desc");
  const [peopleSortKey, setPeopleSortKey] = useState<UserSortKey>(DEFAULT_USER_SORT);
  const [peopleSortDirection, setPeopleSortDirection] = useState<TopSpendersSortDirection>("desc");
  const [searchInput, setSearchInput] = useState(query);
  const [rangeCache, setRangeCache] = useState<{
    account: string;
    paramsKey: string;
    ranges: InsightsResponse["ranges"];
  } | null>(null);
  const debouncedSearchInput = useDebouncedValue(searchInput, SEARCH_DEBOUNCE_MS);

  useEffect(() => {
    setAgentsLimit(DEFAULT_TABLE_LIMIT);
    setPeopleLimit(DEFAULT_TABLE_LIMIT);
  }, [query]);

  useEffect(() => {
    setSearchInput(query);
  }, [query]);

  useEffect(() => {
    if (debouncedSearchInput === query) return;
    onQueryChange(debouncedSearchInput);
  }, [debouncedSearchInput, onQueryChange, query]);

  const baseInsightsParams = useMemo(
    () => buildInsightsQueryParams({
      query,
      agentsLimit,
      peopleLimit,
      agentSortKey,
      agentSortDirection,
      peopleSortKey,
      peopleSortDirection,
    }),
    [agentSortDirection, agentSortKey, agentsLimit, peopleLimit, peopleSortDirection, peopleSortKey, query],
  );
  const baseInsightsParamsKey = useMemo(() => insightsTableParamsSignature(baseInsightsParams), [baseInsightsParams]);
  const cachedRangeState = rangeCache?.account === account ? rangeCache : null;
  const skipRanges = !!cachedRangeState && cachedRangeState.paramsKey !== baseInsightsParamsKey;
  const insightsParams = useMemo(
    () => (skipRanges ? { ...baseInsightsParams, skip_ranges: "true" } : baseInsightsParams),
    [baseInsightsParams, skipRanges],
  );
  const insightsQ = useAccountInsights(account, insightsParams);
  const insights = insightsQ.data;
  const responseRanges = insights?.ranges;
  const hasResponseRanges = responseRanges ? Object.keys(responseRanges).length > 0 : false;

  useEffect(() => {
    if (!account || !responseRanges || Object.keys(responseRanges).length === 0) return;
    setRangeCache({ account, paramsKey: baseInsightsParamsKey, ranges: responseRanges });
  }, [account, baseInsightsParamsKey, responseRanges]);

  const ranges = hasResponseRanges
    ? responseRanges
    : cachedRangeState?.ranges;
  const rangeData = ranges?.[range];
  const days = rangeData?.days ?? RANGE_DAYS[range];
  const agentRows = insights?.tables.agents.rows ?? [];
  const peopleRows = insights?.tables.people.rows ?? [];
  const slackRowsMissingDetails = insights?.tables.people.missing_slack_details_count ?? 0;
  const showSlackDetailsAction = slackRowsMissingDetails > 0 && !insightsQ.isLoading;
  const slackStatusQ = useSlackAccountStatus(account, {
    enabled: showSlackDetailsAction,
  });
  const slackConnect = useSlackAccountConnect(account);
  const slackConnected = (slackStatusQ.data?.workspaces.length ?? 0) > 0;

  const handleAgentSort = (key: AgentSortKey) => {
    setAgentsLimit(DEFAULT_TABLE_LIMIT);
    if (key === agentSortKey) {
      setAgentSortDirection((direction) => (direction === "asc" ? "desc" : "asc"));
      return;
    }
    setAgentSortKey(key);
    setAgentSortDirection("desc");
  };

  const handlePeopleSort = (key: UserSortKey) => {
    setPeopleLimit(DEFAULT_TABLE_LIMIT);
    if (key === peopleSortKey) {
      setPeopleSortDirection((direction) => (direction === "asc" ? "desc" : "asc"));
      return;
    }
    setPeopleSortKey(key);
    setPeopleSortDirection("desc");
  };

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
          usersCount={insights?.tables.people.count || undefined}
          agentsCount={insights?.tables.agents.count || undefined}
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
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
        />
      </div>
    </div>
  );

  const metricsUnavailable =
    insights?.metrics_unavailable === true;

  return (
    <InsightsBody
      range={range}
      displaySummary={rangeData?.stat_cards}
      metricsUnavailable={metricsUnavailable}
      chartLeft={
        <CostOverTimeChart
          data={rangeData?.agent_spend_chart ?? []}
          days={days}
          seriesLabels={rangeData?.series_labels}
          variant={days > 60 ? "line" : "bar"}
        />
      }
      chartRight={<ActiveUsersSpendChart data={rangeData?.people_spend_chart ?? []} days={days} />}
      table={
        view === "agents" ? (
          <TopSpendersTable
            mode="agents"
            rows={agentRows}
            loading={insightsQ.isLoading && !insights}
            groupLabel="Name"
            panelHeader={panelHeader}
            sortKey={agentSortKey}
            sortDirection={agentSortDirection}
            onSort={handleAgentSort}
            pagination={{
              totalRows: insights?.tables.agents.pagination.filtered_count ?? agentRows.length,
              onShowMore: () => setAgentsLimit((limit) => limit + TABLE_PAGE_SIZE),
              onShowLess: () => setAgentsLimit(DEFAULT_TABLE_LIMIT),
            }}
          />
        ) : (
          <TopSpendersTable
            mode="users"
            rows={peopleRows}
            loading={insightsQ.isLoading && !insights}
            panelHeader={panelHeader}
            sortKey={peopleSortKey}
            sortDirection={peopleSortDirection}
            onSort={handlePeopleSort}
            pagination={{
              totalRows: insights?.tables.people.pagination.filtered_count ?? peopleRows.length,
              onShowMore: () => setPeopleLimit((limit) => limit + TABLE_PAGE_SIZE),
              onShowLess: () => setPeopleLimit(DEFAULT_TABLE_LIMIT),
            }}
          />
        )
      }
    />
  );
}
