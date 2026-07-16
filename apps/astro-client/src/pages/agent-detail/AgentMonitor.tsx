import { useState, useMemo, useCallback } from "react";
import { useSearchParams } from "react-router";
import { motion } from "motion/react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { useResolvedTheme } from "@/lib/theme";
import { useAgentDetailContext } from "../AgentDetail";
import {
  useObservabilityMetrics,
  useObservabilityTracesInfinite,
  useObservabilityTraceDetail,
} from "@/api/queries/observability";
import { useNetworkSummary, useNetworkFlows } from "@/api/queries/network";
import { StorageCapacityBanner } from "@/components/StorageCapacityBanner";
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
import { NetworkSummaryCard } from "@/components/agent-detail/network/NetworkSummaryCard";
import { NetworkFlowsTable } from "@/components/agent-detail/network/NetworkFlowsTable";
import type { NetworkDirection, TraceEntry } from "@/lib/api";
import { useContainerSize } from "@/hooks/use-container-size";
import {
  aggregateByLocalDay,
  aggregateRequestsByLocalDay,
} from "@/components/agent-detail/charts/aggregate-token-buckets";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { monitorTracesAnchorId } from "@/lib/routes";

const RANGES: { key: DayRange; label: string; days: number }[] = [
  { key: "7d", label: "7D", days: 7 },
  { key: "14d", label: "14D", days: 14 },
  { key: "30d", label: "30D", days: 30 },
];

const NETWORK_DIRECTIONS: { key: NetworkDirection; label: string }[] = [
  { key: "inbound", label: "Inbound" },
  { key: "outbound", label: "Outbound" },
  { key: "database", label: "Database" },
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
  const [searchParams, setSearchParams] = useSearchParams();
  // Keep the time window in the URL so a shared trace link reproduces the same
  // time context. Fall back to the default for a missing or unknown value.
  const rangeParam = searchParams.get("window");
  const range: DayRange = RANGES.some((r) => r.key === rangeParam)
    ? (rangeParam as DayRange)
    : "7d";
  const setRange = (r: DayRange) => {
    setSearchParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.set("window", r);
        return next;
      },
      { replace: true },
    );
  };
  const { days } = RANGES.find((r) => r.key === range)!;

  const timeParams = useMemo(() => buildTimeParams(days), [days]);
  const { data, isLoading } = useObservabilityMetrics(deploymentId, timeParams, { window: range });

  const traceParams = useMemo(
    () => ({ start_time: timeParams.start_time, end_time: timeParams.end_time }),
    [timeParams],
  );
  const {
    data: tracesData,
    isLoading: tracesLoading,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useObservabilityTracesInfinite(deploymentId, traceParams, { window: range });

  const rawBuckets = data?.buckets ?? [];
  const bars = useMemo(() => aggregateByLocalDay(rawBuckets, days), [rawBuckets, days]);
  const requestPoints = useMemo(() => aggregateRequestsByLocalDay(rawBuckets, days), [rawBuckets, days]);
  const resolvedTheme = useResolvedTheme();
  const colors = resolvedTheme === "dark" ? CHART_COLORS.dark : CHART_COLORS.light;

  const totalInput = bars.reduce((s, b) => s + b.inputTokens, 0);
  const totalOutput = bars.reduce((s, b) => s + b.outputTokens, 0);

  const totalRequests = requestPoints.reduce((s, p) => s + p.requests, 0);

  // Network traffic (Beyla eBPF)
  const [networkDirection, setNetworkDirection] = useState<NetworkDirection>("inbound");
  const networkWindow = useMemo(
    () => ({ from: timeParams.start_time, to: timeParams.end_time }),
    [timeParams.start_time, timeParams.end_time],
  );
  const { data: networkSummary, isLoading: networkSummaryLoading } = useNetworkSummary(
    deploymentId,
    networkWindow,
  );
  const { data: networkFlows, isLoading: networkFlowsLoading } = useNetworkFlows(
    deploymentId,
    networkDirection,
    networkWindow,
  );

  // Trace detail panel
  const allTraces = useMemo(
    () => tracesData?.pages.flatMap((p) => p.traces) ?? [],
    [tracesData],
  );
  const selectedTraceId = searchParams.get("trace");
  const traceFromList = useMemo(
    () => selectedTraceId
      ? allTraces.find((t) => t.trace_id === selectedTraceId) ?? null
      : null,
    [allTraces, selectedTraceId],
  );

  // A deep link (?trace=<id>) can target a trace outside the loaded window
  // (traces are capped at limit:500). When the ID isn't in the list, hydrate a
  // minimal TraceEntry from the detail endpoint so the panel can still open —
  // TraceDetailPanel refetches the same detail (deduped by react-query) for the
  // full body content.
  const needsHydration = !!selectedTraceId && !tracesLoading && !traceFromList;
  const {
    data: hydratedDetail,
    isError: hydrationError,
  } = useObservabilityTraceDetail(
    deploymentId,
    needsHydration ? selectedTraceId : null,
  );

  const hydratedTrace = useMemo<TraceEntry | null>(() => {
    const t = hydratedDetail?.trace;
    if (!needsHydration || !t) return null;
    // The detail endpoint doesn't carry status/total_tokens; the panel defaults
    // status to "success" and sums tokens from observations. input/output here
    // are placeholders — the panel prefers the detail body once it loads.
    return {
      trace_id: t.trace_id,
      name: t.name,
      status: "",
      latency_ms: t.latency_ms,
      total_cost: t.total_cost,
      input: "",
      output: "",
      timestamp: t.timestamp,
      user_id: t.user_id,
      user_details: t.user_details,
    };
  }, [needsHydration, hydratedDetail]);

  const selectedTrace = traceFromList ?? hydratedTrace;

  const selectedIndex = traceFromList
    ? allTraces.findIndex((t) => t.trace_id === traceFromList.trace_id)
    : -1;

  const setSelectedTraceId = useCallback(
    (traceId: string | null, options?: { replace?: boolean }) => {
      setSearchParams((current) => {
        const next = new URLSearchParams(current);
        if (traceId) {
          next.set("trace", traceId);
        } else {
          next.delete("trace");
        }
        return next;
      }, options);
    },
    [setSearchParams],
  );

  const handleSelectTrace = useCallback(
    (trace: TraceEntry) => setSelectedTraceId(trace.trace_id),
    [setSelectedTraceId],
  );

  const handleNavigate = useCallback(
    (dir: "prev" | "next") => {
      const nextIdx = dir === "prev" ? selectedIndex - 1 : selectedIndex + 1;
      if (nextIdx >= 0 && nextIdx < allTraces.length) {
        setSelectedTraceId(allTraces[nextIdx].trace_id, { replace: true });
      }
    },
    [allTraces, selectedIndex, setSelectedTraceId],
  );

  // Track container width for responsive panel behavior
  const { ref: outerRef, width: outerWidth } = useContainerSize();

  const OVERLAY_THRESHOLD = 900;
  const PANEL_WIDTH_REM = 41; // 40rem panel + 1rem gap
  // The panel slides in whenever a trace is requested via the URL — even before
  // it resolves — so a deep link shows a loading or not-found state rather than
  // silently doing nothing while leaving a stale ?trace= behind.
  const traceNotFound = needsHydration && hydrationError;
  const traceHydrating = !!selectedTraceId && !selectedTrace && !traceNotFound;
  const panelOpen = selectedTraceId !== null;
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
          <StorageCapacityBanner deploymentId={deploymentId} className="mb-6" />
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

            <TimeRangeSelector
              value={range}
              ranges={RANGES}
              onChange={(r) => setRange(r as DayRange)}
              layoutId="monitor-range-pill"
            />
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

          {/* Network Traffic */}
          <div className="mt-10">
            <div className="mb-6">
              <h2 className="text-heading-4 text-foreground">Network Traffic</h2>
              <p className="mt-1 text-body-sm text-muted-foreground">
                HTTP traffic to and from your agent
              </p>
            </div>

            <div className="grid grid-cols-1 gap-4 @[540px]/monitor:grid-cols-3">
              <NetworkSummaryCard
                title="Inbound"
                summary={networkSummary?.inbound}
                colors={colors}
                loading={networkSummaryLoading}
              />
              <NetworkSummaryCard
                title="Outbound"
                summary={networkSummary?.outbound}
                colors={colors}
                loading={networkSummaryLoading}
              />
              <NetworkSummaryCard
                title="Database"
                summary={networkSummary?.database}
                colors={colors}
                loading={networkSummaryLoading}
              />
            </div>

            <div className="mt-6 flex items-center justify-between">
              <TimeRangeSelector
                value={networkDirection}
                ranges={NETWORK_DIRECTIONS}
                onChange={(d) => setNetworkDirection(d as NetworkDirection)}
                layoutId="network-direction-pill"
              />
            </div>

            <div className="mt-4">
              <NetworkFlowsTable
                flows={networkFlows?.flows ?? []}
                direction={networkDirection}
                loading={networkFlowsLoading}
              />
            </div>
          </div>

          {/* Traces */}
          <div id={monitorTracesAnchorId} className="mt-10 scroll-mt-6">
            <div className="mb-6">
              <h2 className="text-heading-4 text-foreground">Traces</h2>
            </div>

            <TracesTable
              traces={allTraces}
              account={account}
              loading={tracesLoading}
              selectedTraceId={selectedTraceId}
              onSelectTrace={handleSelectTrace}
              hasMore={hasNextPage}
              onLoadMore={() => fetchNextPage()}
              loadingMore={isFetchingNextPage}
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
        {selectedTrace ? (
          <TraceDetailPanel
            deploymentId={deploymentId}
            trace={selectedTrace}
            account={account}
            onClose={() => setSelectedTraceId(null, { replace: true })}
            canGoPrev={selectedIndex > 0}
            canGoNext={selectedIndex >= 0 && selectedIndex < allTraces.length - 1}
            onNavigate={handleNavigate}
            expanded={panelExpanded}
            onToggleExpanded={shouldOverlay ? undefined : () => setPanelExpanded((v) => !v)}
          />
        ) : traceNotFound ? (
          <div className="flex h-full w-full flex-col items-center justify-center gap-3 rounded-md border border-border bg-surface px-6 text-center">
            <p className="text-body-sm text-foreground">Trace not found.</p>
            <p className="text-body-sm text-muted-foreground">
              It may be outside the selected time range or no longer available.
            </p>
            <button
              type="button"
              onClick={() => setSelectedTraceId(null, { replace: true })}
              className="mt-1 rounded-md border border-border px-3 py-1.5 text-body-sm text-foreground transition-colors hover:bg-muted"
            >
              Close
            </button>
          </div>
        ) : traceHydrating ? (
          <div className="flex h-full w-full items-center justify-center rounded-md border border-border bg-surface">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : null}
      </div>
    </div>
  );
}
