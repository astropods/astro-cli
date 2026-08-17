import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { ArrowUpRight, Bot, Check, ChevronDown, Plus, RefreshCw, TriangleAlert } from "lucide-react";
import { useActiveAccount } from "@/hooks/use-active-account";
import { accountSettingsPath } from "@/lib/settings-paths";
import { useAuth } from "@/lib/auth";
import { cn } from "@/lib/utils";
import { PillToggleChrome } from "@/components/activity/PillToggle";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { ViewToggle, parseActivityView, type ActivityView } from "@/components/activity/ViewToggle";
import { StatCards } from "@/components/activity/StatCards";
import { CostOverTimeChart } from "@/components/activity/CostOverTimeChart";
import { buildModelColorMap, devtoolSourceColor } from "@/components/activity/model-colors";
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
import { dayKeyFromISO } from "@/lib/date-utils";
import { SettledContentReveal } from "@/components/ui/content-reveal";
import { AccountScopeFilter } from "@/components/AccountScopeFilter";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { FilterInput } from "@/components/FilterInput";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuCheckboxItem, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuLabel } from "@/components/ui/dropdown-menu";
import { inputBase, inputFocusVisible } from "@/components/ui/input";
import { getIntegrationIconUrl } from "@/lib/assets";
import { useResolvedTheme } from "@/lib/theme";
import { getPageAccount } from "@/lib/api.server";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { usePersistentSearchParams } from "@/hooks/use-persistent-search-params";
import { observabilityKeys, slackKeys } from "@/api/queries/keys";
import type { InsightsDevtoolSource, InsightsQueryParams, InsightsResponse } from "@/lib/api";
import {
  removeStaleInsightsAccountParam,
  resolveInsightsScopeAccount,
} from "./insights-account-param";
import type { Route } from "./+types/Insights";

const RANGE_DAYS: Record<string, number> = { "7d": 7, "14d": 14, "30d": 30, "90d": 90 };
const SLACK_OAUTH_PARAMS = ["slack_connected", "slack_user", "slack_team", "slack_error"] as const;
const SLACK_REFRESH_FEEDBACK_MS = 3_500;
const SEARCH_DEBOUNCE_MS = 300;
const DEFAULT_TABLE_LIMIT = 25;
const TABLE_PAGE_SIZE = 10;
const SHOW_TOP_LABEL = `Show top ${DEFAULT_TABLE_LIMIT}`;
const DEFAULT_AGENT_SORT: AgentSortKey = "cost_usd";
const DEFAULT_USER_SORT: UserSortKey = "cost_usd";
const INSIGHTS_FILTER_PARAMS = [
  "range",
  "view",
  "hide_sources",
] as const;

// Dev-tool usage folds into the agent surfaces as distinct series/rows keyed by
// source; "agents" is the base deployed-agent source in the Sources filter.
const AGENTS_SOURCE_KEY = "agents";

function parseHiddenSources(raw: string | null): Set<string> {
  return new Set((raw ?? "").split(",").map((s) => s.trim()).filter(Boolean));
}

type SlackRefreshStatus = "idle" | "refreshing" | "success" | "error";

function stripSlackOAuthParams(params: URLSearchParams) {
  for (const key of SLACK_OAUTH_PARAMS) params.delete(key);
  return params;
}

// The window the server reported, once a response exists. Tagged with the range
// it belongs to so a range switch falls back to the local estimate for one frame
// instead of briefly labelling the new range with the old range's dates.
export interface ResolvedWindow {
  range: ActivityRange;
  from: string;
  to: string;
}

// The header label prefers the window the server reported and falls back to a
// locally-inferred one. The range check is the point: the reported window
// arrives an effect after the range chip flips, and labelling the new range
// with the old range's dates is worse than briefly showing an estimate.
export function resolveInsightsDateLabel(
  resolved: ResolvedWindow | null,
  range: ActivityRange,
): string {
  if (resolved?.range !== range) return buildDateLabel(range);
  return `${formatDateShort(resolved.from)} – ${formatDateShort(resolved.to)}`;
}

// Usage is totalled per completed day, so the page ends at the last of them
// rather than implying today is included-but-empty. Name the day whenever the
// server reported one; without it the window is still complete-days-only, we
// just can't say which day it reaches.
export function insightsFreshnessNote(asOf?: string): string {
  if (!asOf) return "Usage is totalled once a day, so today's activity may not appear yet.";
  return `Usage is totalled once a day. Showing everything through ${formatDateShort(asOf)}.`;
}

// Fallback only, for the first paint before any response has arrived: a window
// ending today, which is what the client can infer on its own. The real label
// comes from the response — see ResolvedWindow — because the reported window
// ends where the data ends, and on the rollup-backed path that is the last
// complete day rather than today.
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

export function buildInsightsQueryParams({
  query = "",
  agentsLimit = DEFAULT_TABLE_LIMIT,
  peopleLimit = DEFAULT_TABLE_LIMIT,
  agentSortKey = DEFAULT_AGENT_SORT,
  agentSortDirection = "desc",
  peopleSortKey = DEFAULT_USER_SORT,
  peopleSortDirection = "desc",
  skipRanges = false,
  hideSources = [],
  days,
}: {
  query?: string;
  agentsLimit?: number;
  peopleLimit?: number;
  agentSortKey?: AgentSortKey;
  agentSortDirection?: TopSpendersSortDirection;
  peopleSortKey?: UserSortKey;
  peopleSortDirection?: TopSpendersSortDirection;
  skipRanges?: boolean;
  hideSources?: string[];
  days?: number;
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
    hide_sources: hideSources.length ? [...hideSources].sort().join(",") : undefined,
    days: days ? String(days) : undefined,
  };
}

// Identifies a table-only param set: a change to any of these refetches with
// skip_ranges, leaving the charts on the cached ranges.
//
// hide_sources is intentionally EXCLUDED — it changes the ranges (chart/stat
// cards), so toggling a source must trigger a full-ranges refetch. `days` IS
// included, and the distinction is the point: one response carries every range,
// so changing the range never invalidates the charts, only which window the
// tables cover. Add a param here only if it leaves the ranges untouched.
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
    params.days ?? "",
  ].join("\u0000");
}

export async function loader({ request }: Route.LoaderArgs) {
  const ctx = await getPageAccount(request);
  if (!ctx) {
    return { account: null, insights: null, insightsParams: buildInsightsQueryParams({}) };
  }

  const url = new URL(request.url);
  // `days` has to be read from the URL, not defaulted: the page sends the
  // selected range's window and primes on the exact param set, so omitting it
  // here would make every first paint refetch instead of hitting the primed
  // entry.
  const insightsParams = buildInsightsQueryParams({
    query: url.searchParams.get("q") ?? "",
    hideSources: [...parseHiddenSources(url.searchParams.get("hide_sources"))],
    days: RANGE_DAYS[parseRange(url.searchParams.get("range"))],
  });
  const insights = await ctx.api.getAccountInsights(ctx.accountName, insightsParams).catch(() => null);
  return { account: ctx.accountName, insights, insightsParams };
}

// Range, view, and search changes stay client-side. A page-local account
// change re-runs the loader so that account's first Insights response is
// server-rendered and primes the exact TanStack key used below.
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
    return currentUrl.searchParams.get("account") !== nextUrl.searchParams.get("account");
  }
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
  const { accounts } = useAuth();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] =
    usePersistentSearchParams("insights", INSIGHTS_FILTER_PARAMS);
  const range = parseRange(searchParams.get("range"));
  const view = parseActivityView(searchParams.get("view"));
  const q = searchParams.get("q") ?? "";
  // `hide_sources` lists the Sources-filter keys to exclude (a source or
  // "agents"); absent = all on.
  const hiddenSources = useMemo(() => parseHiddenSources(searchParams.get("hide_sources")), [searchParams]);
  const [devtoolSources, setDevtoolSources] = useState<InsightsDevtoolSource[]>([]);
  const handleDevtoolSources = useCallback((sources: InsightsDevtoolSource[]) => setDevtoolSources(sources), []);
  const [resolvedWindow, setResolvedWindow] = useState<ResolvedWindow | null>(null);
  const handleResolvedWindow = useCallback((w: ResolvedWindow | null) => setResolvedWindow(w), []);
  const resolvedTheme = useResolvedTheme();
  const sourceOptions = useMemo(
    () => [
      { key: AGENTS_SOURCE_KEY, label: "Astro AI agents", icon: <img src={getIntegrationIconUrl("astro", resolvedTheme)} alt="" className="size-4 shrink-0 object-contain" /> },
      ...devtoolSources.map((src) => ({
        key: src.key,
        label: src.label,
        icon: src.icon
          ? <img src={getIntegrationIconUrl(src.icon, resolvedTheme)} alt="" className="size-4 shrink-0 object-contain" />
          : <Bot className="size-4 shrink-0 text-muted-foreground" aria-hidden />,
      })),
    ],
    [devtoolSources, resolvedTheme],
  );
  const paramAccount = searchParams.get("account");
  const accountNames = useMemo(() => accounts.map((account) => account.name), [accounts]);
  const scopeAccount = resolveInsightsScopeAccount(paramAccount, accountNames, activeAccount);
  useEffect(() => {
    const next = removeStaleInsightsAccountParam(searchParams, accountNames);
    if (next) setSearchParams(next, { replace: true });
  }, [accountNames, paramAccount, searchParams, setSearchParams]);
  const setScopeAccount = useCallback((next: string) => {
    setSearchParams((previous) => {
      if (next === activeAccount) previous.delete("account");
      else previous.set("account", next);
      return previous;
    }, { replace: true });
  }, [activeAccount, setSearchParams]);
  // "Add a source" links to this account's Data Sources settings and hotlinks
  // the create modal open (see ApiKeysSettings' ?new= handling).
  const dataSourcesHref = useMemo(
    () => `${accountSettingsPath(accounts, scopeAccount, "api-keys")}?new=1`,
    [accounts, scopeAccount],
  );
  // Only offer "Add a source" when the user could actually create one — the
  // ingest-key create endpoint requires account manage rights (personal
  // accounts, or org admins/owners). Mirrors the Vault "+ New" gating.
  const canAddDataSource = useMemo(() => {
    const acct = accounts.find((a) => a.name === scopeAccount);
    return !acct || acct.type === "personal" || acct.role === "admin" || acct.role === "owner";
  }, [accounts, scopeAccount]);
  const slackConnected = searchParams.get("slack_connected") === "true";
  const slackError = searchParams.get("slack_error");
  const hasSlackOAuthParam = SLACK_OAUTH_PARAMS.some((key) => searchParams.has(key));
  const accountForSlackResync = scopeAccount || loaderData.account || "";
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

  const toggleSource = useCallback((key: string) => {
    setSearchParams((prev) => {
      const set = parseHiddenSources(prev.get("hide_sources"));
      if (set.has(key)) set.delete(key);
      else set.add(key);
      if (set.size === 0) prev.delete("hide_sources");
      else prev.set("hide_sources", [...set].join(","));
      return prev;
    }, { replace: true });
  }, [setSearchParams]);

  const dateLabel = resolveInsightsDateLabel(resolvedWindow, range);

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
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label="Filter sources"
            className={cn(
              "flex h-8 items-center gap-2 px-2.5 text-sm leading-none text-foreground transition-colors !bg-white dark:!bg-transparent hover:!bg-slate-50 dark:hover:!bg-slate-800",
              inputBase,
              inputFocusVisible,
            )}
          >
            Sources
            <ChevronDown className="size-4 shrink-0 opacity-50" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          {/* Agents (always present) */}
          {sourceOptions.slice(0, 1).map((opt) => (
            <DropdownMenuCheckboxItem
              key={opt.key}
              checked={!hiddenSources.has(opt.key)}
              onCheckedChange={() => toggleSource(opt.key)}
              onSelect={(e) => e.preventDefault()}
            >
              {opt.icon}
              {opt.label}
            </DropdownMenuCheckboxItem>
          ))}
          {/* External — always shown, even with no external sources yet */}
          <DropdownMenuSeparator />
          <DropdownMenuLabel className="text-xs font-medium text-muted-foreground">External</DropdownMenuLabel>
          {sourceOptions.slice(1).map((opt) => (
            <DropdownMenuCheckboxItem
              key={opt.key}
              checked={!hiddenSources.has(opt.key)}
              onCheckedChange={() => toggleSource(opt.key)}
              onSelect={(e) => e.preventDefault()}
            >
              {opt.icon}
              {opt.label}
            </DropdownMenuCheckboxItem>
          ))}
          {canAddDataSource && (
            <DropdownMenuItem onSelect={() => navigate(dataSourcesHref)}>
              <Plus className="size-4 shrink-0" />
              Add a source
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <TimeRangeSelector
        value={range}
        onChange={(r) => setSearchParams((prev) => { prev.set("range", r); return prev; }, { replace: true })}
      />
    </div>
  );

  return (
    <PageContainer outerClassName="bg-background">
      <PageHeader
        title="Insights for"
        adornment={
          <AccountScopeFilter
            value={scopeAccount}
            onChange={setScopeAccount}
            className="-ml-1"
          />
        }
        description="Track usage, cost, and reliability for this account."
        action={headerAction}
      />

      <InsightsView
        account={scopeAccount}
        range={range}
        view={view}
        onViewChange={setView}
        query={q}
        onQueryChange={setQuery}
        onDevtoolSources={handleDevtoolSources}
        onResolvedWindow={handleResolvedWindow}
        hiddenSources={hiddenSources}
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
  // Last day the data is complete through, when the server reported one.
  asOf?: string;
}

function InsightsBody({ range, displaySummary, chartLeft, chartRight, table, asOf }: InsightsBodyProps) {
  return (
    <>
      <StatCards data={displaySummary} showChange range={range} />
      <div>
        <div className="mb-6 grid grid-cols-1 gap-4 @xl:grid-cols-2">
          <div className="h-[300px]">{chartLeft}</div>
          <div className="h-[300px]">{chartRight}</div>
        </div>
        {table}
      </div>
      {/* Keep it muted — it's a disclaimer, not a status. */}
      <p className="mt-6 text-center text-body-sm text-faint-foreground">
        {insightsFreshnessNote(asOf)}
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
  onDevtoolSources: (sources: InsightsDevtoolSource[]) => void;
  onResolvedWindow: (w: ResolvedWindow | null) => void;
  hiddenSources: Set<string>;
  slackRefreshStatus: SlackRefreshStatus;
}

function InsightsView({
  account,
  range,
  view,
  onViewChange,
  query,
  onQueryChange,
  onDevtoolSources,
  onResolvedWindow,
  hiddenSources,
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
      hideSources: [...hiddenSources],
      days: RANGE_DAYS[range],
    }),
    [agentSortDirection, agentSortKey, agentsLimit, peopleLimit, peopleSortDirection, peopleSortKey, query, hiddenSources, range],
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

  // Both charts and the header label anchor to the window the server reported
  // rather than to the clock. On the rollup-backed path that window ends at the
  // last complete day, so anchoring on today appended a bucket the facts can
  // never fill — an empty trailing bar that read as "we lost your data".
  const windowStart = dayKeyFromISO(rangeData?.period?.start);
  const windowEnd = dayKeyFromISO(rangeData?.period?.end);
  useEffect(() => {
    onResolvedWindow(
      windowStart && windowEnd ? { range, from: windowStart, to: windowEnd } : null,
    );
  }, [onResolvedWindow, range, windowStart, windowEnd]);
  const agentRows = insights?.tables.agents.rows ?? [];
  const peopleRows = insights?.tables.people.rows ?? [];

  // Dev-tool usage (Claude Code, Codex, …) is folded into the chart, stat cards,
  // and tables server-side; the client renders what the server returns. Chart
  // series for dev-tool sources get a distinct brand color; agents use the model
  // palette.
  const chartColorMap = useMemo(() => {
    const modelKeys = [...new Set((rangeData?.agent_spend_chart ?? []).flatMap((d) => d.models.map((m) => m.model)))];
    const sourceKeys = new Set((insights?.devtool_sources ?? []).map((s) => s.key));
    const map = buildModelColorMap(modelKeys.filter((k) => !sourceKeys.has(k)));
    let i = 0;
    for (const k of modelKeys) {
      if (sourceKeys.has(k)) map[k] = devtoolSourceColor(k, i++);
    }
    return map;
  }, [rangeData, insights?.devtool_sources]);

  const displaySummary = rangeData?.stat_cards;
  const agentsTotalRows = insights?.tables.agents.pagination.filtered_count ?? agentRows.length;
  const peopleTotalRows = insights?.tables.people.pagination.filtered_count ?? peopleRows.length;

  // Report the account's dev-tool sources up for the (parent-rendered) filter.
  // Report [] when absent so switching to an account with no dev-tool usage
  // clears the previous account's entries rather than leaving them stale.
  const responseDevtoolSources = insights?.devtool_sources;
  useEffect(() => {
    onDevtoolSources(responseDevtoolSources ?? []);
  }, [responseDevtoolSources, onDevtoolSources]);
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

  // The Models view is a per-model breakdown, not a name-searchable list, so it
  // hides the search box and Slack action; only the toggle stays.
  const isModelView = view === "models";

  // Counts shown in the toggle pills are the un-filtered totals — the pill
  // reflects how much data exists, not how much the current search returns.
  // Toggle + search render inside the table's bordered container via the
  // Table primitive's `header` slot.
  const panelHeader = (
    <div className="flex flex-col gap-3 @md:flex-row @md:items-center @md:justify-between">
      <div className="flex items-center gap-3">
        <ViewToggle
          value={view}
          onChange={onViewChange}
          usersCount={insights?.tables.people.count || undefined}
          agentsCount={insights?.tables.agents.count || undefined}
        />
      </div>
      {!isModelView && (
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
      )}
    </div>
  );

  return (
    <SettledContentReveal
      transitionKey={account}
      settled={
        insights !== undefined &&
        !insightsQ.isPlaceholderData &&
        !insightsQ.isError
      }
    >
      <InsightsBody
        range={range}
        displaySummary={displaySummary}
        asOf={insights?.as_of}
        chartLeft={
          <CostOverTimeChart
            data={rangeData?.agent_spend_chart ?? []}
            days={days}
            endDate={windowEnd}
            colorMap={chartColorMap}
            seriesLabels={rangeData?.series_labels}
            variant={days > 60 ? "line" : "bar"}
          />
        }
        chartRight={
          <ActiveUsersSpendChart
            data={rangeData?.people_spend_chart ?? []}
            days={days}
            endDate={windowEnd}
          />
        }
        table={
          isModelView ? (
            <TopSpendersTable mode="models" account={account} days={days} panelHeader={panelHeader} />
          ) : view === "agents" ? (
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
                totalRows: agentsTotalRows,
                defaultVisibleRows: DEFAULT_TABLE_LIMIT,
                pageSize: TABLE_PAGE_SIZE,
                showLessLabel: SHOW_TOP_LABEL,
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
                totalRows: peopleTotalRows,
                defaultVisibleRows: DEFAULT_TABLE_LIMIT,
                pageSize: TABLE_PAGE_SIZE,
                showLessLabel: SHOW_TOP_LABEL,
                onShowMore: () => setPeopleLimit((limit) => limit + TABLE_PAGE_SIZE),
                onShowLess: () => setPeopleLimit(DEFAULT_TABLE_LIMIT),
              }}
            />
          )
        }
      />
    </SettledContentReveal>
  );
}
