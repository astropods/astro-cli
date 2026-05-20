import { useMemo } from "react";
import { useSearchParams } from "react-router";
import { motion } from "motion/react";
import { ChartBarIcon } from "@heroicons/react/24/outline";
import { useActiveAccount } from "@/hooks/use-active-account";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { AgentFilterBar } from "@/components/activity/AgentFilterBar";
import { StatCards } from "@/components/activity/StatCards";
import { CostOverTimeChart } from "@/components/activity/CostOverTimeChart";
import { TopSpendersTable } from "@/components/activity/TopSpendersTable";
import { useInsightsData } from "@/components/activity/use-insights-data";
import { buildPeriodParams, type ActivityRange } from "@/components/activity/ranges";
import { observabilityKeys } from "@/api/queries/keys";
import { formatDateShort } from "@/lib/format-utils";
import { dashboardPath } from "@/lib/routes";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageHeader } from "@/components/PageLayout";
import { EmptyState } from "@/components/EmptyState";
import { PageStarField } from "@/components/agent-detail/starfield/PageStarField";
import { getActiveAccount } from "@/lib/api.server";
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
  if (!ctx) return { account: null, summary: null, blueprintsData: null, from: null, to: null };

  const url = new URL(request.url);
  const range = parseRange(url.searchParams.get("range"));
  const { from, to } = buildPeriodParams(range);
  const params: Record<string, string> = {};
  if (from) params.from = from;
  if (to) params.to = to;

  const [summary, blueprintsData] = await Promise.all([
    ctx.api.getAccountObservabilitySummary(ctx.accountName, params).catch(() => null),
    ctx.api.getAccountBlueprintsSummary(ctx.accountName, params).catch(() => null),
  ]);

  return {
    account: ctx.accountName,
    summary,
    blueprintsData,
    from: from ?? null,
    to: to ?? null,
  };
}

// Range/agent toggles change search params client-side — only org switches
// (programmatic revalidations, currentUrl === nextUrl) need the loader.
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
  const { activeAccount } = useActiveAccount();
  const [searchParams, setSearchParams] = useSearchParams();
  const range = parseRange(searchParams.get("range"));
  const rawAgents = searchParams.get("agents");
  const selectedAgents = rawAgents ? rawAgents.split(",").filter(Boolean) : [];

  function setSelectedAgents(agents: string[]) {
    setSearchParams((prev) => {
      if (agents.length === 0) prev.delete("agents");
      else prev.set("agents", agents.join(","));
      return prev;
    }, { replace: true });
  }

  // Prefer the loader's timestamps (the ones the SSR data is keyed under);
  // fall back to client-computed window on CSR range toggles where the
  // loader didn't re-run.
  const clientWindow = useMemo(() => buildPeriodParams(range), [range]);
  const from = loaderData?.from ?? clientWindow.from;
  const to = loaderData?.to ?? clientWindow.to;

  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (!ld?.account) return;
    if (ld.summary) {
      qc.setQueryData(observabilityKeys.activitySummary(ld.account, from, to), ld.summary);
    }
    if (ld.blueprintsData) {
      qc.setQueryData(observabilityKeys.blueprintsSummary(ld.account, from, to), ld.blueprintsData);
    }
  });

  const {
    allAgentNames,
    filteredBlueprints,
    agentCostOverTime,
    displaySummary,
    allAgentColorMap,
    activeColorMap,
    isLoading,
    hasData,
  } = useInsightsData({
    account: activeAccount,
    selectedAgents,
    from,
    to,
  });

  const dateLabel = buildDateLabel(range, from, to);
  const days = RANGE_DAYS[range];

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

          <div className="mb-6">
            <div className="grid grid-cols-1 @md:grid-cols-3 items-center gap-3">
              <div className="col-span-2">
                <AgentFilterBar
                  value={selectedAgents}
                  onValueChange={setSelectedAgents}
                  allAgentNames={allAgentNames}
                  colorMap={allAgentColorMap}
                />
              </div>
              <div className="flex items-center justify-end gap-3">
                {dateLabel && (
                  <span className="font-mono text-body-sm text-muted-foreground">{dateLabel}</span>
                )}
                <TimeRangeSelector
                  value={range}
                  onChange={(r) => setSearchParams((prev) => { prev.set("range", r); return prev; }, { replace: true })}
                />
              </div>
            </div>
          </div>

          <motion.div
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.18 }}
          >
            <StatCards
              data={displaySummary}
              showChange={range !== "all"}
              range={range}
            />
          </motion.div>

          {!isLoading && !hasData ? (
            <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
              <EmptyState
                variant="card"
                icon={<ChartBarIcon className="mx-auto size-10 text-faint-foreground" />}
                title="No insights for this period"
                description="Deploy agents and start sending requests to see usage data here."
                actions={[{ label: "Go to Agents", to: dashboardPath }]}
              />
            </motion.div>
          ) : (
            <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
              <div className="mb-6 h-[300px]">
                <CostOverTimeChart
                  data={agentCostOverTime}
                  days={days}
                  colorMap={activeColorMap}
                  seriesLabels={{ __all__: "All Agents" }}
                  variant={range === "all" ? "line" : "bar"}
                />
              </div>

              <TopSpendersTable
                blueprints={filteredBlueprints}
                groupLabel="Agent"
              />
            </motion.div>
          )}
        </div>
      </div>
    </div>
  );
}
