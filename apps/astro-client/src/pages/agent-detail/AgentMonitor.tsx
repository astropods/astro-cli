import { useEffect, useState, useMemo } from "react";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import { useResolvedTheme } from "@/lib/theme";
import { useAgentDetailContext } from "../AgentDetail";
import { useObservabilityMetrics } from "@/api/queries/observability";
import { useNetworkSummary, useNetworkFlows } from "@/api/queries/network";
import { StorageCapacityBanner } from "@/components/StorageCapacityBanner";
import { ContentReveal } from "@/components/ui/content-reveal";
import { TokenUsageChart } from "@/components/agent-detail/charts/TokenUsageChart";
import {
  CHART_COLORS,
  DAY_RANGES,
  buildTimeParams,
  formatCompactNumber,
  type DayRange,
} from "@/components/agent-detail/charts/chart-utils";
import { RequestVolumeChart } from "@/components/agent-detail/charts/RequestVolumeChart";
import { LatencyCard } from "@/components/agent-detail/charts/LatencyCard";
import { NetworkSummaryCard } from "@/components/agent-detail/network/NetworkSummaryCard";
import { NetworkFlowsTable } from "@/components/agent-detail/network/NetworkFlowsTable";
import { NetworkFlowGraph } from "@/components/agent-detail/network/NetworkFlowGraph";
import { useDeploymentAvatarUrl } from "@/lib/avatar-bust";
import type { NetworkDirection, NetworkFlow } from "@/lib/api";
import {
  aggregateByLocalDay,
  aggregateRequestsByLocalDay,
} from "@/components/agent-detail/charts/aggregate-token-buckets";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { deploymentPath, DeploymentTab } from "@/lib/routes";
import { classify } from "@/components/agent-detail/pods/classify";
import { cn } from "@/lib/utils";

// Stable identity, or a pending query re-runs the graph's layout every render.
const NO_FLOWS: NetworkFlow[] = [];

const CORE_NETWORK_DIRECTIONS: { key: NetworkDirection; label: string }[] = [
  { key: "inbound", label: "Inbound" },
  { key: "outbound", label: "Outbound" },
];
const DATABASE_NETWORK_DIRECTION: { key: NetworkDirection; label: string } = {
  key: "database",
  label: "Database",
};

export default function AgentMonitor() {
  const { deployment, deploymentId, account } = useAgentDetailContext();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const legacyTraceId = searchParams.get("trace");
  const legacyTracesAnchor = location.hash === "#traces";
  useEffect(() => {
    if (!legacyTraceId && !legacyTracesAnchor) return;
    const next = new URLSearchParams();
    if (legacyTraceId) next.set("trace", legacyTraceId);
    const windowParam = searchParams.get("window");
    if (windowParam) next.set("window", windowParam);
    const query = next.toString();
    navigate(
      `${deploymentPath(account, deploymentId, DeploymentTab.Traces)}${
        query ? `?${query}` : ""
      }`,
      { replace: true },
    );
  }, [
    account,
    deploymentId,
    legacyTraceId,
    legacyTracesAnchor,
    navigate,
    searchParams,
  ]);
  // Keep the metrics time window in the URL so shared monitor links preserve
  // their context. Fall back to the default for a missing or unknown value.
  const rangeParam = searchParams.get("window");
  const range: DayRange = DAY_RANGES.some((r) => r.key === rangeParam)
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
  const { days } = DAY_RANGES.find((r) => r.key === range)!;

  const timeParams = useMemo(
    () => buildTimeParams(days, { granularity: "hour" }),
    [days],
  );
  const { data, isLoading } = useObservabilityMetrics(deploymentId, timeParams, { window: range });

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
  const hasKnowledgeConfigured = (deployment.workloads ?? []).some(
    (workload) =>
      classify(workload.component, workload.kind) === "knowledge" ||
      Object.values(workload.env ?? {}).some((variables) =>
        variables.some((variable) => variable.source === "knowledge_cred"),
      ),
  );
  const networkDirections = useMemo(
    () =>
      hasKnowledgeConfigured
        ? [...CORE_NETWORK_DIRECTIONS, DATABASE_NETWORK_DIRECTION]
        : CORE_NETWORK_DIRECTIONS,
    [hasKnowledgeConfigured],
  );

  useEffect(() => {
    if (networkDirection === "database" && !hasKnowledgeConfigured) {
      setNetworkDirection("inbound");
    }
  }, [hasKnowledgeConfigured, networkDirection]);

  const { data: networkFlows, isLoading: networkFlowsLoading } = useNetworkFlows(
    deploymentId,
    networkDirection,
    networkWindow,
  );

  // No sort/limit, so the table's active direction shares a key and is cached.
  const { data: inboundFlows } = useNetworkFlows(deploymentId, "inbound", networkWindow);
  const { data: outboundFlows } = useNetworkFlows(deploymentId, "outbound", networkWindow);
  const graphInbound = inboundFlows?.flows ?? NO_FLOWS;
  const graphOutbound = outboundFlows?.flows ?? NO_FLOWS;
  const hasGraphTraffic = graphInbound.length > 0 || graphOutbound.length > 0;
  const agentAvatarUrl = useDeploymentAvatarUrl(deployment);

  return (
    <div className="relative z-10 flex flex-1 overflow-hidden pt-16">
      {/* Main content */}
      <div
        className="relative z-10 min-h-0 flex-1 overflow-y-auto"
        style={{
          maskImage: "linear-gradient(to bottom, transparent, black 2rem)",
          WebkitMaskImage: "linear-gradient(to bottom, transparent, black 2rem)",
        }}
      >
        <ContentReveal
          className="@container/monitor mx-auto w-full max-w-4xl px-6 py-8 pb-16"
        >
          <StorageCapacityBanner deploymentId={deploymentId} className="mb-6" />
          {/* Header */}
          <div className="mb-6 flex items-end justify-between">
            <div>
              <h2 className="text-heading-4 text-foreground">Token Usage</h2>
              {!isLoading && (
                <p className="mt-1 text-body-sm text-foreground dark:text-muted-foreground">
                  {formatCompactNumber(totalInput)} input · {formatCompactNumber(totalOutput)} output
                </p>
              )}
            </div>

            <TimeRangeSelector
              value={range}
              ranges={DAY_RANGES}
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
                <p className="mt-1 text-body-sm text-foreground dark:text-muted-foreground">
                  {formatCompactNumber(totalRequests)} total requests
                </p>
              )}
            </div>

            <div className="grid grid-cols-1 gap-4 @[540px]/monitor:grid-cols-3">
              <div className="min-h-[300px] @[540px]/monitor:col-span-2">
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
              <p className="mt-1 text-body-sm text-foreground dark:text-muted-foreground">
                HTTP traffic to and from your agent
              </p>
            </div>

            {hasGraphTraffic && (
              <NetworkFlowGraph
                inbound={graphInbound}
                outbound={graphOutbound}
                agentAvatarUrl={agentAvatarUrl}
                className="mb-8"
              />
            )}

            <div
              className={cn(
                "grid grid-cols-1 gap-4",
                hasKnowledgeConfigured
                  ? "@[540px]/monitor:grid-cols-3"
                  : "@[540px]/monitor:grid-cols-2",
              )}
            >
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
              {hasKnowledgeConfigured && (
                <NetworkSummaryCard
                  title="Database"
                  summary={networkSummary?.database}
                  colors={colors}
                  loading={networkSummaryLoading}
                />
              )}
            </div>

            <div className="mt-6 flex items-center justify-between">
              <TimeRangeSelector
                value={networkDirection}
                ranges={networkDirections}
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
        </ContentReveal>
      </div>
    </div>
  );
}
