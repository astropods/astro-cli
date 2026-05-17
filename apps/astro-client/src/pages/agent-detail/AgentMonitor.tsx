import { useState, useMemo, useCallback } from "react";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";
import { useResolvedTheme } from "@/lib/theme";
import { useAgentDetailContext } from "../AgentDetail";
import {
  useObservabilityMetrics,
  useObservabilityTraces,
} from "@/api/queries/observability";
import { TokenUsageChart } from "@/components/agent-detail/charts/TokenUsageChart";
import {
  CHART_COLORS,
  formatCompactNumber,
  type DayRange,
} from "@/components/agent-detail/charts/chart-utils";
import { RequestVolumeChart } from "@/components/agent-detail/charts/RequestVolumeChart";
import { LatencyCard } from "@/components/agent-detail/charts/LatencyCard";
import { TracesTable } from "@/components/agent-detail/traces/TracesTable";
import { TraceDetailPanel } from "@/components/agent-detail/traces/TraceDetailPanel";
import type { TraceEntry } from "@/lib/api";
import { useContainerSize } from "@/hooks/use-container-size";
import {
  aggregateByLocalDay,
  aggregateRequestsByLocalDay,
} from "@/components/agent-detail/charts/aggregate-token-buckets";

const RANGES: { key: DayRange; label: string; days: number }[] = [
  { key: "7d", label: "7D", days: 7 },
  { key: "14d", label: "14D", days: 14 },
  { key: "30d", label: "30D", days: 30 },
];

function buildTimeParams(days: number) {
  const end = new Date();
  const start = new Date(end);
  start.setDate(start.getDate() - days);
  return {
    start_time: start.toISOString(),
    end_time: end.toISOString(),
    granularity: "hour",
  };
}

export default function AgentMonitor() {
  const { deploymentId, account } = useAgentDetailContext();
  const [range, setRange] = useState<DayRange>("7d");
  const { days } = RANGES.find((r) => r.key === range)!;

  const timeParams = useMemo(() => buildTimeParams(days), [days]);
  const { data, isLoading } = useObservabilityMetrics(deploymentId, timeParams, { window: range });

  const traceParams = useMemo(
    () => ({ start_time: timeParams.start_time, end_time: timeParams.end_time, limit: "100" }),
    [timeParams],
  );
  const { data: tracesData, isLoading: tracesLoading } = useObservabilityTraces(
    deploymentId,
    traceParams,
    { window: range },
  );

  const rawBuckets = data?.buckets ?? [];
  const bars = useMemo(() => aggregateByLocalDay(rawBuckets, days), [rawBuckets, days]);
  const requestPoints = useMemo(() => aggregateRequestsByLocalDay(rawBuckets, days), [rawBuckets, days]);
  const resolvedTheme = useResolvedTheme();
  const colors = resolvedTheme === "dark" ? CHART_COLORS.dark : CHART_COLORS.light;

  const totalInput = bars.reduce((s, b) => s + b.inputTokens, 0);
  const totalOutput = bars.reduce((s, b) => s + b.outputTokens, 0);

  const totalRequests = requestPoints.reduce((s, p) => s + p.requests, 0);

  // Trace detail panel
  const [selectedTrace, setSelectedTrace] = useState<TraceEntry | null>(null);
  const allTraces = tracesData?.traces ?? [];

  const selectedIndex = selectedTrace
    ? allTraces.findIndex((t) => t.trace_id === selectedTrace.trace_id)
    : -1;

  const handleNavigate = useCallback(
    (dir: "prev" | "next") => {
      const nextIdx = dir === "prev" ? selectedIndex - 1 : selectedIndex + 1;
      if (nextIdx >= 0 && nextIdx < allTraces.length) {
        setSelectedTrace(allTraces[nextIdx]);
      }
    },
    [selectedIndex, allTraces],
  );

  // Track container width for responsive panel behavior
  const { ref: outerRef, width: outerWidth } = useContainerSize();

  const OVERLAY_THRESHOLD = 900;
  const PANEL_WIDTH_REM = 41; // 40rem panel + 1rem gap
  const panelOpen = selectedTrace !== null;
  const shouldOverlay = outerWidth > 0 && outerWidth < OVERLAY_THRESHOLD;
  const [panelExpanded, setPanelExpanded] = useState(false);
  const isFullWidth = panelExpanded || shouldOverlay;

  return (
    <div ref={outerRef} className="relative z-10 flex flex-1 overflow-hidden pt-16">
      {/* Main content */}
      <div
        className="relative z-10 min-h-0 flex-1 overflow-y-auto transition-[padding] duration-300 ease-out"
        style={{
          paddingRight: panelOpen && !isFullWidth ? `${PANEL_WIDTH_REM}rem` : undefined,
          maskImage: "linear-gradient(to bottom, transparent, black 2rem)",
          WebkitMaskImage: "linear-gradient(to bottom, transparent, black 2rem)",
        }}
      >
        <motion.div
          className="@container/monitor mx-auto w-full max-w-4xl px-6 py-8 pb-16"
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.25, ease: "easeOut" }}
        >
          {/* Header */}
          <div className="mb-6 flex items-end justify-between">
            <div>
              <h2 className="text-heading-4 text-foreground">Token Usage</h2>
              {!isLoading && (
                <p className="mt-1 text-body-sm text-muted-foreground">
                  {formatCompactNumber(totalInput)} input · {formatCompactNumber(totalOutput)} output
                </p>
              )}
            </div>

            {/* Range picker */}
            <div className="flex items-center rounded-md border border-border bg-surface/60 p-0.5">
              {RANGES.map(({ key, label }) => (
                <button
                  key={key}
                  onClick={() => setRange(key)}
                  aria-pressed={key === range}
                  className={cn(
                    "relative rounded-[10px] px-3 py-1 text-mono-sm font-medium transition-colors",
                    key === range ? "text-foreground" : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {key === range && (
                    <motion.div
                      layoutId="monitor-range-pill"
                      className="absolute inset-0 rounded-[10px] bg-primary/15 dark:bg-white/10"
                      transition={{ type: "spring", bounce: 0.15, duration: 0.4 }}
                    />
                  )}
                  <span className="relative">{label}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Chart */}
          <TokenUsageChart
            bars={bars}
            colors={colors}
            loading={isLoading}
          />

          {/* Requests & Latency */}
          <div className="mt-10">
            <div className="mb-6">
              <h2 className="text-heading-4 text-foreground">Requests &amp; Latency</h2>
              {!isLoading && (
                <p className="mt-1 text-body-sm text-muted-foreground">
                  {formatCompactNumber(totalRequests)} total requests
                </p>
              )}
            </div>

            <div className="grid grid-cols-1 gap-4 @[540px]/monitor:grid-cols-3">
              <div className="min-h-[300px] @[540px]/monitor:col-span-2 @[540px]/monitor:min-h-0">
                <RequestVolumeChart
                  points={requestPoints}
                  colors={colors}
                  loading={isLoading}
                />
              </div>
              <div className="col-span-1">
                <LatencyCard
                  points={requestPoints}
                  colors={colors}
                  loading={isLoading}
                />
              </div>
            </div>
          </div>

          {/* Traces */}
          <div className="mt-10">
            <div className="mb-6">
              <h2 className="text-heading-4 text-foreground">Traces</h2>
            </div>

            <TracesTable
              traces={allTraces}
              account={account}
              loading={tracesLoading}
              selectedTraceId={selectedTrace?.trace_id}
              onSelectTrace={setSelectedTrace}
            />
          </div>
        </motion.div>
      </div>

      {/* Trace detail panel */}
      <div
        className={cn(
          "absolute z-20 transition-[transform,inset,width] duration-300 ease-out",
          isFullWidth
            ? "inset-3 top-20"
            : "bottom-3 right-3 top-20 w-[40rem]",
        )}
        style={{ transform: panelOpen ? "translateX(0)" : "translateX(calc(100% + 0.75rem))" }}
      >
        {selectedTrace && (
          <TraceDetailPanel
            deploymentId={deploymentId}
            trace={selectedTrace}
            onClose={() => setSelectedTrace(null)}
            canGoPrev={selectedIndex > 0}
            canGoNext={selectedIndex < allTraces.length - 1}
            onNavigate={handleNavigate}
            expanded={panelExpanded}
            onToggleExpanded={shouldOverlay ? undefined : () => setPanelExpanded((v) => !v)}
          />
        )}
      </div>
    </div>
  );
}
