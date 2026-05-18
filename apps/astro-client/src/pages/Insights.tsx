import { useSearchParams } from "react-router";
import { motion } from "motion/react";
import { ChartBarIcon } from "@heroicons/react/24/outline";
import { useActiveAccount } from "@/hooks/use-active-account";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { AgentFilterBar } from "@/components/activity/AgentFilterBar";
import { StatCards } from "@/components/activity/StatCards";
import { CostOverTimeChart } from "@/components/activity/CostOverTimeChart";
import { TopSpendersTable } from "@/components/activity/TopSpendersTable";
import { useInsightsData } from "@/components/activity/use-insights-data";
import { buildPeriodParams, type ActivityRange } from "@/components/activity/ranges";
import { formatDateShort } from "@/lib/format-utils";
import { dashboardPath } from "@/lib/routes";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageHeader } from "@/components/PageLayout";
import { EmptyState } from "@/components/EmptyState";
import { PageStarField } from "@/components/agent-detail/starfield/PageStarField";
import { getPersonalAccount } from "@/lib/api.server";
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
  const ctx = await getPersonalAccount(request);
  if (!ctx) return { summary: null, blueprintsData: null, blueprintCount: 0 };

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

  return { summary, blueprintsData, blueprintCount: blueprintsData?.blueprints?.length ?? 0, from: from ?? null, to: to ?? null };
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

  const {
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
    isLoading,
    hasData,
  } = useInsightsData({
    account: activeAccount,
    range,
    selectedAgents,
    initialSummary: loaderData?.summary,
    initialBlueprintsData: loaderData?.blueprintsData,
    ssrFrom: loaderData?.from,
    ssrTo: loaderData?.to,
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
              loading={summaryLoading}
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
                  loading={blueprintsLoading}
                  days={days}
                  colorMap={activeColorMap}
                  seriesLabels={{ __all__: "All Agents" }}
                  variant={range === "all" ? "line" : "bar"}
                />
              </div>

              <TopSpendersTable
                blueprints={filteredBlueprints}
                loading={blueprintsLoading}
                groupLabel="Agent"
              />
            </motion.div>
          )}
        </div>
      </div>
    </div>
  );
}
