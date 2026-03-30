import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ResponsiveContainer, ComposedChart, Area, Line, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { Activity, Copy, Check, X } from "lucide-react";
import { ChevronDownIcon, ChevronUpIcon } from "@heroicons/react/24/outline";
import { mapDeploymentStatus } from "@/lib/deployment-utils";
import { useObservabilityMetrics, useObservabilitySummary, useObservabilityTraces } from "@/api/queries/observability";
import { observabilityKeys } from "@/api/queries/keys";
import { api } from "@/lib/api";
import type { AgentDeployment } from "@/lib/api";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { InlineBadge } from "@/components/InlineBadge";
import { Button } from "@/components/ui/button";
import { MultiSelect } from "../shared/MultiSelect";
import { HeadlineMetrics, type WindowTrend } from "./HeadlineMetrics";
import { buildPreviousWindowParams, percentChange } from "./trend-utils";
import { copyTextToClipboard } from "@/lib/clipboard";

const C = {
  bg: "var(--muted)",
  bgAlt: "var(--surface)",
  bgDeep: "var(--muted)",
  panel: "var(--surface)",
  border: "var(--border)",
  teal: "var(--primary)",
  tealMid: "var(--color-teal-600)",
  tealLt: "var(--color-teal-400)",
  text: "var(--foreground)",
  muted: "var(--muted-foreground)",
  faint: "var(--faint-foreground)",
  stone: "var(--color-stone-500)",
  amber: "var(--color-amber-700)",
  issue: "var(--color-yellow-500)",
  amberBg: "color-mix(in oklch, var(--color-amber-700) 12%, transparent)",
  amberBdr: "color-mix(in oklch, var(--color-amber-700) 28%, transparent)",
  coral: "var(--color-coral-600)",
  coralBg: "color-mix(in oklch, var(--color-coral-600) 12%, transparent)",
  coralBdr: "color-mix(in oklch, var(--color-coral-600) 28%, transparent)",
  success: "var(--color-green-700)",
} as const;

const S = {
  body: "var(--font-sans), sans-serif",
  mono: "var(--font-mono), monospace",
} as const;

const T = {
  heading2: "var(--text-heading-2)",
  heading1: "var(--text-heading-1)",
  heading4: "var(--text-heading-4)",
  body: "var(--text-body)",
  bodySm: "var(--text-body-sm)",
  label: "var(--text-label)",
  monoSm: "var(--text-mono-sm)",
  monoMd: "var(--text-mono-md)",
} as const;

const I = {
  xs: 10,
  sm: 12,
  md: 14,
  lg: 16,
} as const;

export type TraceStatus = "success" | "error" | "timeout";

export interface TraceRow {
  id: string;
  name: string;
  status: TraceStatus;
  latency: number;
  time: string;
  tokens: number;
  input?: string;
  output?: string;
}

export const TRACE_STATUS_STYLE: Record<TraceStatus, { label: string; badgeStyle: CSSProperties }> = {
  success: {
    label: "Success",
    badgeStyle: {
      color: "var(--color-teal-600)",
      background: "color-mix(in oklch, var(--color-teal-600) 12%, transparent)",
    },
  },
  error: {
    label: "Error",
    badgeStyle: {
      color: "var(--color-red-700)",
      background: "color-mix(in oklch, var(--color-red-700) 12%, transparent)",
    },
  },
  timeout: {
    label: "Timeout",
    badgeStyle: {
      color: "var(--color-yellow-700)",
      background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)",
    },
  },
};

function fmtTokens(n: number) {
  return Math.round(n).toLocaleString();
}

export function formatLatencyMs(ms: number): string {
  if (!Number.isFinite(ms)) return "—";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function middleEllipsis(value: string, edge = 5): string {
  if (!value) return value;
  const minLengthForTruncation = edge * 2 + 3;
  if (value.length <= minLengthForTruncation) return value;
  return `${value.slice(0, edge)}...${value.slice(-edge)}`;
}

const CHART_H = 130;
const TRACE_GRID_COLUMNS = "250px 1fr 80px 80px 80px 132px";

interface ChartTooltipProps {
  active?: boolean;
  payload?: { name: string; value: number; color: string }[];
  label?: string;
  reqVisible: boolean;
  avgLatVisible: boolean;
}

function Ghost({ width = "100%", height = 12, radius = 6 }: { width?: string | number; height?: number; radius?: number }) {
  return (
    <span
      className="dp-pulse"
      style={{
        display: "inline-block",
        width,
        height,
        borderRadius: radius,
        background: `linear-gradient(90deg, ${C.bgDeep} 0%, ${C.border} 45%, ${C.bgDeep} 100%)`,
      }}
    />
  );
}

function ChartTooltip({ active, payload, label, reqVisible, avgLatVisible }: ChartTooltipProps) {
  if (!active || !payload?.length) return null;
  const req = payload.find((p) => p.name === "req");
  const avgLat = payload.find((p) => p.name === "avgLat");
  return (
    <div
      style={{
        background: C.panel,
        border: `1px solid ${C.border}`,
        borderRadius: 7,
        padding: "6px 10px",
        boxShadow: "0 2px 10px rgba(0,0,0,0.1)",
        pointerEvents: "none",
      }}
    >
      <div style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, marginBottom: 4, letterSpacing: "0.06em" }}>
        {label}
      </div>
      {reqVisible && req && (
        <div style={{ display: "flex", alignItems: "center", gap: 5, fontFamily: S.mono, fontSize: T.monoSm }}>
          <span style={{ width: 6, height: 6, borderRadius: 1, background: C.tealMid, display: "inline-block", flexShrink: 0 }} />
          <span style={{ color: C.tealMid, fontWeight: 600 }}>{req.value}</span>
          <span style={{ color: C.faint }}>req</span>
        </div>
      )}
      {avgLatVisible && avgLat && (
        <div style={{ display: "flex", alignItems: "center", gap: 5, fontFamily: S.mono, fontSize: T.monoSm, marginTop: 2 }}>
          <span style={{ width: 8, height: 1.5, background: C.issue, display: "inline-block", flexShrink: 0 }} />
          <span style={{ color: C.issue, fontWeight: 600 }}>{formatLatencyMs(avgLat.value)}</span>
          <span style={{ color: C.faint }}>avg</span>
        </div>
      )}
    </div>
  );
}

function InlineChart({
  data,
  reqVisible,
  avgLatVisible,
  win,
  hideXAxisLabels,
  animate = true,
}: {
  data: { t: string; req: number; avgLatencyMs: number }[];
  reqVisible: boolean;
  avgLatVisible: boolean;
  win: "1h" | "24h" | "7d";
  hideXAxisLabels: boolean;
  animate?: boolean;
}) {
  if (data.length === 0) return null;
  const hasSinglePoint = data.length === 1;
  const reqMax = Math.max(...data.map((d) => d.req));
  const reqUpper = Math.max(1, Math.ceil(reqMax * 1.15));
  const latMin = Math.min(...data.map((d) => d.avgLatencyMs));
  const latMax = Math.max(...data.map((d) => d.avgLatencyMs));
  const latPad = (latMax - latMin) * 0.15 || 50;
  const latLower = Math.max(0, Math.floor(latMin - latPad));
  const latUpper = Math.ceil(latMax + latPad);
  return (
    <ResponsiveContainer width="100%" height={CHART_H}>
      <ComposedChart data={data} margin={{ top: 8, right: 14, bottom: 0, left: 12 }}>
        <defs>
          <linearGradient id="req-grad-obs" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={C.tealMid} stopOpacity="0.18" />
            <stop offset="95%" stopColor={C.tealMid} stopOpacity="0.02" />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke={C.border} strokeOpacity={0.6} vertical={false} />
        <XAxis
          dataKey="t"
          tick={{ fontFamily: S.mono, fontSize: T.monoSm, fill: C.faint }}
          tickLine={false}
          axisLine={false}
          interval={hideXAxisLabels ? "preserveStartEnd" : 0}
          minTickGap={hideXAxisLabels ? 56 : 24}
          padding={{ left: 10, right: 10 }}
          tickMargin={6}
          tickFormatter={(value, index) => {
            if (hideXAxisLabels) return "";
            if (index === 0) return "";
            if (win === "24h") {
              return index % 4 === 0 ? String(value) : "";
            }
            return String(value);
          }}
        />
        <YAxis
          yAxisId="req"
          orientation="left"
          domain={[0, reqUpper]}
          tick={{ fontFamily: S.mono, fontSize: T.monoSm, fill: C.faint }}
          tickLine={false}
          axisLine={false}
          width={48}
          tickFormatter={(v: number) => (v >= 1000 ? `${(v / 1000).toFixed(0)}k` : String(v))}
          tickCount={4}
        />
        <YAxis
          yAxisId="ms"
          orientation="right"
          domain={[latLower, latUpper]}
          tick={{ fontFamily: S.mono, fontSize: T.monoSm, fill: C.faint }}
          tickLine={false}
          axisLine={false}
          width={52}
          tickFormatter={(v: number) => formatLatencyMs(v)}
          tickCount={4}
        />
        <Tooltip
          content={<ChartTooltip reqVisible={reqVisible} avgLatVisible={avgLatVisible} />}
          cursor={{ stroke: C.stone, strokeWidth: 0.8, strokeDasharray: "2 2" }}
        />
        {reqVisible && (
          <Area
            yAxisId="req"
            dataKey="req"
            name="req"
            stroke={C.tealMid}
            strokeWidth={1.5}
            fill="url(#req-grad-obs)"
            dot={hasSinglePoint ? { r: 3.5, fill: C.tealMid, stroke: C.panel, strokeWidth: 1.5 } : false}
            activeDot={{ r: 4, fill: C.tealMid, stroke: C.panel, strokeWidth: 1.5 }}
            isAnimationActive={animate}
            animationDuration={1000}
            animationEasing="ease-out"
          />
        )}
        {avgLatVisible && (
          <Line
            yAxisId="ms"
            dataKey="avgLatencyMs"
            name="avgLat"
            stroke={C.issue}
            strokeWidth={1.5}
            strokeDasharray="4 3"
            strokeOpacity={0.85}
            dot={hasSinglePoint ? { r: 3.5, fill: C.issue, stroke: C.panel, strokeWidth: 1.5 } : false}
            activeDot={{ r: 4, fill: C.issue, stroke: C.panel, strokeWidth: 1.5 }}
            isAnimationActive={animate}
            animationDuration={1000}
            animationEasing="ease-out"
          />
        )}
      </ComposedChart>
    </ResponsiveContainer>
  );
}

const WIN_HOURS: Record<string, number> = { "1h": 1, "24h": 24, "7d": 168 };
type Win = "1h" | "24h" | "7d";
const OBS_WINDOWS: Win[] = ["1h", "24h", "7d"];
const WIN_BUCKETS: Record<"1h" | "24h" | "7d", number> = { "1h": 6, "24h": 24, "7d": 7 };
const WIN_BUCKET_MS: Record<"1h" | "24h" | "7d", number> = {
  "1h": 10 * 60 * 1000,
  "24h": 60 * 60 * 1000,
  "7d": 24 * 60 * 60 * 1000,
};

function buildTimeParams(win: string) {
  const hours = WIN_HOURS[win] ?? 24;
  const end = new Date();
  const start = new Date(end.getTime() - hours * 60 * 60 * 1000);
  return { start_time: start.toISOString(), end_time: end.toISOString() };
}

function clampWindowParams(
  params: { start_time: string; end_time: string },
  minStartTime?: string,
): { start_time: string; end_time: string } {
  if (!minStartTime) return params;
  const minMs = new Date(minStartTime).getTime();
  const startMs = new Date(params.start_time).getTime();
  const endMs = new Date(params.end_time).getTime();
  if (!Number.isFinite(minMs) || Number.isNaN(minMs)) return params;
  const clampedStart = Math.max(startMs, minMs);
  const clampedEnd = Math.max(endMs, minMs);
  return {
    start_time: new Date(clampedStart).toISOString(),
    end_time: new Date(clampedEnd).toISOString(),
  };
}


function buildRequestVolumeSeries(
  traces: Array<{ timestamp: string; latency_ms: number }>,
  win: "1h" | "24h" | "7d",
  endTimeISO: string
): { t: string; req: number; avgLatencyMs: number }[] {
  const bucketCount = WIN_BUCKETS[win];
  const bucketMs = WIN_BUCKET_MS[win];
  const endMs = new Date(endTimeISO).getTime();
  const startMs = endMs - bucketCount * bucketMs;

  const buckets = Array.from({ length: bucketCount }, (_, idx) => ({
    startMs: startMs + idx * bucketMs,
    req: 0,
    totalLatencyMs: 0,
  }));

  for (const trace of traces) {
    const ts = new Date(trace.timestamp).getTime();
    if (Number.isNaN(ts) || ts < startMs || ts > endMs) continue;
    const idx = Math.min(bucketCount - 1, Math.floor((ts - startMs) / bucketMs));
    const bucket = buckets[idx];
    bucket.req += 1;
    bucket.totalLatencyMs += trace.latency_ms;
  }

  return buckets.map((b) => {
    const d = new Date(b.startMs);
    const label =
      win === "7d"
        ? d.toLocaleDateString([], { month: "short", day: "numeric" })
        : d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

    return {
      t: label,
      req: b.req,
      avgLatencyMs: b.req > 0 ? b.totalLatencyMs / b.req : 0,
    };
  });
}

export function MonitorTab({ deployment, selectedTraceId, onSelectTrace, onVisibleTracesChange }: {
  deployment: AgentDeployment;
  selectedTraceId?: string | null;
  onSelectTrace?: (trace: TraceRow) => void;
  onVisibleTracesChange?: (traces: TraceRow[]) => void;
}) {
  const queryClient = useQueryClient();
  const [isCompact, setIsCompact] = useState<boolean>(() => {
    if (typeof window === "undefined") return false;
    return window.innerWidth < 1180;
  });
  const [showAllTraces, setShowAllTraces] = useState(false);
  const [isObservabilityNoticeDismissed, setIsObservabilityNoticeDismissed] = useState(false);
  const [win, setWin] = useState<Win>("24h");
  const [traceStatuses, setTraceStatuses] = useState<string[]>([]);
  const [copiedTraceId, setCopiedTraceId] = useState<string | null>(null);
  const [series, setSeries] = useState({ req: true, avgLat: true });
  const [tokenView, setTokenView] = useState<"input" | "output">("input");
  const [windowParams] = useState<Record<Win, { start_time: string; end_time: string }>>(() => ({
    "1h": buildTimeParams("1h"),
    "24h": buildTimeParams("24h"),
    "7d": buildTimeParams("7d"),
  }));
  const deploymentStartIso = useMemo(() => {
    const ms = new Date(deployment.created_at).getTime();
    return Number.isFinite(ms) && !Number.isNaN(ms) ? new Date(ms).toISOString() : undefined;
  }, [deployment.created_at]);
  const scopedWindowParams = useMemo<Record<Win, { start_time: string; end_time: string }>>(
    () => ({
      "1h": clampWindowParams(windowParams["1h"], deploymentStartIso),
      "24h": clampWindowParams(windowParams["24h"], deploymentStartIso),
      "7d": clampWindowParams(windowParams["7d"], deploymentStartIso),
    }),
    [windowParams, deploymentStartIso],
  );
  const previousWindowParams = useMemo(() => {
    const prev = buildPreviousWindowParams(scopedWindowParams);
    return {
      "1h": clampWindowParams(prev["1h"], deploymentStartIso),
      "24h": clampWindowParams(prev["24h"], deploymentStartIso),
      "7d": clampWindowParams(prev["7d"], deploymentStartIso),
    } as Record<Win, { start_time: string; end_time: string }>;
  }, [scopedWindowParams, deploymentStartIso]);
  const timeParams = scopedWindowParams[win];

  useEffect(() => {
    if (!deployment.id) return;

    for (const w of OBS_WINDOWS) {
      const params = scopedWindowParams[w];
      const prevParams = previousWindowParams[w];
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.metrics(deployment.id, w),
        queryFn: () => api.getObservabilityMetrics(deployment.id, params),
      });
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.summary(deployment.id, w),
        queryFn: () => api.getObservabilitySummary(deployment.id, params),
      });
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.traces(deployment.id, w),
        queryFn: () => api.getObservabilityTraces(deployment.id, { ...params, limit: "100" }),
      });
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.summary(deployment.id, `prev-${w}`),
        queryFn: () => api.getObservabilitySummary(deployment.id, prevParams),
      });
    }
  }, [deployment.id, queryClient, scopedWindowParams, previousWindowParams]);

  const metricsQuery = useObservabilityMetrics(deployment.id, timeParams, { window: win });
  const tracesQuery = useObservabilityTraces(deployment.id, { ...timeParams, limit: "100" }, { window: win });
  const summary1hQuery = useObservabilitySummary(deployment.id, scopedWindowParams["1h"], { window: "1h" });
  const summary24hQuery = useObservabilitySummary(deployment.id, scopedWindowParams["24h"], { window: "24h" });
  const summary7dQuery = useObservabilitySummary(deployment.id, scopedWindowParams["7d"], { window: "7d" });
  const prevSummary1hQuery = useObservabilitySummary(deployment.id, previousWindowParams["1h"], { window: "prev-1h" });
  const prevSummary24hQuery = useObservabilitySummary(deployment.id, previousWindowParams["24h"], { window: "prev-24h" });
  const prevSummary7dQuery = useObservabilitySummary(deployment.id, previousWindowParams["7d"], { window: "prev-7d" });

  const summaryByWin: Record<Win, typeof summary1hQuery> = {
    "1h": summary1hQuery,
    "24h": summary24hQuery,
    "7d": summary7dQuery,
  };
  const prevSummaryByWin: Record<Win, typeof prevSummary1hQuery> = {
    "1h": prevSummary1hQuery,
    "24h": prevSummary24hQuery,
    "7d": prevSummary7dQuery,
  };
  const selectedSummaryQuery = summaryByWin[win];
  const selectedPrevSummaryQuery = prevSummaryByWin[win];
  const { data: metricsData } = metricsQuery;
  const { data: summaryData } = selectedSummaryQuery;
  const { data: tracesData } = tracesQuery;
  // Skip chart entry animation when data was already cached (remount),
  // so a background refetch doesn't interrupt the animation mid-way.
  const mountedWithCache = useRef(!!tracesData);
  const animateChart = !mountedWithCache.current;
  const observabilityBackendError = metricsQuery.isError || selectedSummaryQuery.isError || tracesQuery.isError;
  const tracesLoading = tracesQuery.isLoading && !tracesData;

  const bucketTokenTotals = useMemo(() => {
    let input = 0;
    let output = 0;
    for (const b of metricsData?.buckets ?? []) {
      input += b.input_tokens ?? 0;
      output += b.output_tokens ?? 0;
    }
    return { input, output, sum: input + output };
  }, [metricsData]);

  const traces: TraceRow[] = (tracesData?.traces ?? []).map((t) => ({
    id: t.trace_id,
    name: t.name,
    status: t.status === "error" || t.status === "failed" ? "error" : t.status === "timeout" ? "timeout" : "success",
    latency: t.latency_ms,
    time: new Date(t.timestamp).toLocaleString([], {
      year: "numeric",
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }),
    tokens: t.total_tokens ?? 0,
    input: typeof t.input === "string" ? t.input : t.input != null ? JSON.stringify(t.input, null, 2) : undefined,
    output: typeof t.output === "string" ? t.output : t.output != null ? JSON.stringify(t.output, null, 2) : undefined,
  }));

  const tsData = useMemo(
    () =>
      buildRequestVolumeSeries(
        (tracesData?.traces ?? []).map((t) => ({ timestamp: t.timestamp, latency_ms: t.latency_ms })),
        win,
        timeParams.end_time
      ),
    [tracesData, win, timeParams.end_time]
  );

  const visibleTraces = useMemo(
    () => traces.filter((t) => {
      if (traceStatuses.length > 0 && !traceStatuses.includes(t.status)) return false;
      return true;
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [tracesData, traceStatuses],
  );

  useEffect(() => {
    onVisibleTracesChange?.(visibleTraces);
  }, [visibleTraces, onVisibleTracesChange]);

  const copyTraceId = (id: string) => {
    void copyTextToClipboard(id);
    setCopiedTraceId(id);
    setTimeout(() => setCopiedTraceId((prev) => (prev === id ? null : prev)), 1200);
  };


  const summaryLoading = selectedSummaryQuery.isLoading && !summaryData;
  const trendLoading =
    (selectedSummaryQuery.isLoading && !selectedSummaryQuery.data) ||
    (selectedPrevSummaryQuery.isLoading && !selectedPrevSummaryQuery.data);

  const trends: Record<"1h" | "24h" | "7d", WindowTrend> = useMemo(
    () => ({
      "1h": {
        total_traces: percentChange(summary1hQuery.data?.total_traces, prevSummary1hQuery.data?.total_traces),
        error_rate: percentChange(summary1hQuery.data?.metrics.error_rate, prevSummary1hQuery.data?.metrics.error_rate),
        avg_latency_ms: percentChange(summary1hQuery.data?.metrics.avg_latency_ms, prevSummary1hQuery.data?.metrics.avg_latency_ms),
        p95_latency_ms: percentChange(summary1hQuery.data?.metrics.p95_latency_ms, prevSummary1hQuery.data?.metrics.p95_latency_ms),
      },
      "24h": {
        total_traces: percentChange(summary24hQuery.data?.total_traces, prevSummary24hQuery.data?.total_traces),
        error_rate: percentChange(summary24hQuery.data?.metrics.error_rate, prevSummary24hQuery.data?.metrics.error_rate),
        avg_latency_ms: percentChange(summary24hQuery.data?.metrics.avg_latency_ms, prevSummary24hQuery.data?.metrics.avg_latency_ms),
        p95_latency_ms: percentChange(summary24hQuery.data?.metrics.p95_latency_ms, prevSummary24hQuery.data?.metrics.p95_latency_ms),
      },
      "7d": {
        total_traces: percentChange(summary7dQuery.data?.total_traces, prevSummary7dQuery.data?.total_traces),
        error_rate: percentChange(summary7dQuery.data?.metrics.error_rate, prevSummary7dQuery.data?.metrics.error_rate),
        avg_latency_ms: percentChange(summary7dQuery.data?.metrics.avg_latency_ms, prevSummary7dQuery.data?.metrics.avg_latency_ms),
        p95_latency_ms: percentChange(summary7dQuery.data?.metrics.p95_latency_ms, prevSummary7dQuery.data?.metrics.p95_latency_ms),
      },
    }),
    [summary1hQuery.data, summary24hQuery.data, summary7dQuery.data, prevSummary1hQuery.data, prevSummary24hQuery.data, prevSummary7dQuery.data]
  );

  const tokenLoading = metricsQuery.isLoading && !metricsData;
  const tokenSplitFromBuckets = bucketTokenTotals.sum > 0;
  const tokens = {
    input: tokenSplitFromBuckets ? bucketTokenTotals.input : 0,
    output: tokenSplitFromBuckets ? bucketTokenTotals.output : 0,
    total: bucketTokenTotals.sum,
    hasSplit: tokenSplitFromBuckets,
  };
  const activeToken =
    !tokens.hasSplit && tokens.total > 0
      ? { value: tokens.total, color: C.text }
      : tokenView === "input"
        ? { value: tokens.input, color: C.text }
        : { value: tokens.output, color: C.text };

  const isError = mapDeploymentStatus(deployment) === "error";
  const traceGridColumns = isCompact
    ? "minmax(0, 1.25fr) minmax(0, 0.72fr) minmax(0, 0.72fr) minmax(0, 0.72fr)"
    : TRACE_GRID_COLUMNS;
  const traceGridGap = isCompact ? 8 : 10;
  const traceHeaderPadding = isCompact ? "7px 10px" : "7px 16px";
  const traceRowPadding = isCompact ? "10px 10px" : "10px 16px";
  const traceHeaderFontSize = isCompact ? "11px" : T.label;
  const traceCellFontSize = "12px";
  const tracesEmptyMinHeight = 172;
  const hasCollapsedTraces = visibleTraces.length > 4;
  const visibleTraceRows = showAllTraces ? visibleTraces : visibleTraces.slice(0, 4);

  useEffect(() => {
    const onResize = () => setIsCompact(window.innerWidth < 1180);
    onResize();
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, []);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {isError && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: "10px 16px",
            borderRadius: 8,
            background: C.coralBg,
            border: `1px solid ${C.coralBdr}`,
          }}
        >
          <span style={{ fontFamily: S.mono, fontSize: T.label, fontWeight: 700, letterSpacing: "0.08em", color: C.coral }}>
            ERROR
          </span>
          <span style={{ fontFamily: S.body, fontSize: T.body, color: C.coral, flex: 1 }}>
            This deployment is in an error state — no replicas are ready.
          </span>
        </div>
      )}

      {observabilityBackendError && !isError && !isObservabilityNoticeDismissed && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            padding: "12px 16px",
            borderRadius: 8,
            background: C.amberBg,
            border: `1px solid ${C.amberBdr}`,
          }}
        >
          <span
            style={{
              fontFamily: S.mono,
              fontSize: T.label,
              fontWeight: 700,
              letterSpacing: "0.08em",
              color: C.amber,
              display: "inline-flex",
              alignItems: "center",
              whiteSpace: "nowrap",
              lineHeight: 1.1,
            }}
          >
            OBSERVABILITY
          </span>
          <span style={{ fontFamily: S.body, fontSize: T.body, color: C.muted, flex: 1, lineHeight: 1.45 }}>
            Trace metrics are temporarily unavailable. You can still inspect runtime and pod logs on the{" "}
            <strong style={{ color: C.text }}>Deployments</strong> tab.
          </span>
          <button
            type="button"
            onClick={() => setIsObservabilityNoticeDismissed(true)}
            aria-label="Dismiss observability notice"
            style={{
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              width: 24,
              height: 24,
              borderRadius: 6,
              border: `1px solid ${C.coral}`,
              background: "transparent",
              color: C.coral,
              cursor: "pointer",
              flexShrink: 0,
            }}
          >
            <X size={14} />
          </button>
        </div>
      )}

      <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <span style={{ fontFamily: S.body, fontSize: T.heading1, fontWeight: 600, color: C.text }}>Monitor</span>
            <Select value={win} onValueChange={(value) => setWin(value as Win)}>
              <SelectTrigger
                className="h-9 w-auto min-w-[160px] px-3"
                style={{ fontFamily: S.body, fontSize: T.body, background: "var(--popover)" }}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="1h">Last 1 hour</SelectItem>
                <SelectItem value="24h">Last 24 hours</SelectItem>
                <SelectItem value="7d">Last 7 days</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <HeadlineMetrics
            summary={summaryData ? { total_traces: summaryData.total_traces, metrics: summaryData.metrics } : null}
            summaryLoading={summaryLoading}
            trendLoading={trendLoading}
            selectedWindow={win}
            trends={trends}
          />

          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))", gap: 12 }}>
            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ marginBottom: 8 }}>
                <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 700, color: C.text }}>Request volume</span>
              </div>
              <div style={{ display: "flex", gap: 10, marginBottom: 6 }}>
                {[
                  { key: "req" as const, color: C.tealMid, label: "Requests", dashed: false },
                  { key: "avgLat" as const, color: C.issue, label: "Avg latency", dashed: true },
                ].map((s) => (
                  <button
                    key={s.key}
                    onClick={() => setSeries((p) => ({ ...p, [s.key]: !p[s.key] }))}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 4,
                      background: "none",
                      border: "none",
                      cursor: "pointer",
                      padding: 0,
                      opacity: series[s.key] ? 1 : 0.35,
                      transition: "opacity 0.15s",
                    }}
                  >
                    <span
                      style={{
                        width: 18,
                        height: 2,
                        background: s.dashed ? "none" : s.color,
                        backgroundImage: s.dashed
                          ? `repeating-linear-gradient(to right, ${s.color} 0, ${s.color} 4px, transparent 4px, transparent 7px)`
                          : "none",
                        display: "inline-block",
                        borderRadius: 1,
                        flexShrink: 0,
                      }}
                    />
                    <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>{s.label}</span>
                  </button>
                ))}
              </div>
              {tsData.length === 0 ? (
                <div style={{ height: 130, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 10, textAlign: "center" }}>
                  <div style={{ width: 32, height: 32, borderRadius: "50%", background: C.bgDeep, display: "flex", alignItems: "center", justifyContent: "center" }}>
                    <Activity size={I.md} color={C.stone} />
                  </div>
                  <div>
                    <p style={{ fontFamily: S.body, fontSize: T.body, fontWeight: 600, color: C.text, margin: "0 0 3px" }}>No requests yet</p>
                    <p style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, margin: 0, letterSpacing: "0.03em" }}>
                      Volume will appear once traffic starts
                    </p>
                  </div>
                </div>
              ) : (
                <InlineChart
                  key={win}
                  data={tsData}
                  reqVisible={series.req}
                  avgLatVisible={series.avgLat}
                  win={win}
                  hideXAxisLabels={isCompact}
                  animate={animateChart}
                />
              )}
            </div>

            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 14, overflow: "hidden" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 18px", borderBottom: `1px solid ${C.border}` }}>
                <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 700, color: C.text }}>Token usage</span>
                <div style={{ transform: "scale(0.9)", transformOrigin: "right center" }}>
                  <ToggleGroup
                    type="single"
                    variant="word"
                    className="w-auto"
                    value={tokenView}
                    onValueChange={(v) => {
                      if (v) setTokenView(v as "input" | "output");
                    }}
                  >
                    <ToggleGroupItem value="input" disabled={!tokens.hasSplit} aria-label="Input tokens">
                      Input
                    </ToggleGroupItem>
                    <ToggleGroupItem value="output" disabled={!tokens.hasSplit} aria-label="Output tokens">
                      Output
                    </ToggleGroupItem>
                  </ToggleGroup>
                </div>
              </div>
              <div style={{ padding: "16px 18px 14px" }}>
                <div style={{ overflow: "hidden", lineHeight: 1.2 }}>
                  <span
                    key={`${win}-${tokenView}-${tokens.hasSplit}`}
                    className="dp-slot-in"
                    style={{
                      display: "block",
                      fontFamily: S.body,
                      fontSize: T.heading1,
                      fontWeight: 700,
                      color: activeToken.color,
                      letterSpacing: "-0.02em",
                    }}
                  >
                    {fmtTokens(activeToken.value)}
                  </span>
                </div>
                {tokenLoading ? (
                  <p style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, margin: "10px 0 0", lineHeight: 1.5 }}>Loading token usage...</p>
                ) : (
                  <>
                    <div style={{ display: "flex", height: 12, borderRadius: 999, overflow: "hidden", margin: "14px 0 10px", background: "rgba(196,184,158,0.8)" }}>
                      <div style={{ flex: tokens.input || 1, background: C.tealMid, opacity: tokenView === "input" ? (tokens.total === 0 ? 0.2 : 1) : 0.35, transition: "opacity 0.2s" }} />
                      <div style={{ flex: tokens.output || 1, background: C.issue, opacity: tokenView === "output" ? (tokens.total === 0 ? 0.2 : 1) : 0.35, transition: "opacity 0.2s" }} />
                    </div>
                    <div style={{ display: "flex", justifyContent: "space-between" }}>
                      <div style={{ overflow: "hidden", lineHeight: 1.3 }}>
                        <span key={win} className="dp-slot-in" style={{ display: "block", fontFamily: S.body, fontSize: T.bodySm, color: C.faint }}>
                          {tokens.total > 0 ? `of ${fmtTokens(tokens.total)} total` : "No token usage yet"}
                        </span>
                      </div>
                      {tokens.total > 0 && (
                        <div style={{ overflow: "hidden", lineHeight: 1.3 }}>
                          <span key={`${win}-pct`} className="dp-slot-in" style={{ display: "block", fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>
                            {Math.round((activeToken.value / tokens.total) * 100)}%
                          </span>
                        </div>
                      )}
                    </div>
                  </>
                )}
              </div>
            </div>
          </div>

          <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "visible", display: "flex", flexDirection: "column" }}>
            <div style={{ display: "flex", alignItems: "center", flexWrap: isCompact ? "wrap" : "nowrap", gap: 10, padding: "12px 16px", borderBottom: `1px solid ${C.border}`, flexShrink: 0 }}>
              <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 700, color: C.text, flex: 1 }}>Traces</span>
              <MultiSelect
                options={[
                  { value: "success", label: "Success", color: C.success },
                  { value: "error", label: "Error", color: C.coral },
                  { value: "timeout", label: "Timeout", color: C.issue },
                ]}
                selected={traceStatuses}
                onChange={setTraceStatuses}
                placeholder="All statuses"
              />
            </div>
            <div style={{ minHeight: 0 }}>
              <div style={{ display: "flex", flexDirection: "column" }}>
                <div style={{ display: "grid", gridTemplateColumns: traceGridColumns, gap: traceGridGap, padding: traceHeaderPadding, borderBottom: `1px solid ${C.border}`, background: C.bgDeep, flexShrink: 0 }}>
                  <span style={{ fontFamily: S.mono, fontSize: traceHeaderFontSize, letterSpacing: "0.07em", color: C.faint, textAlign: "left", minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>DATE</span>
                  {(isCompact ? ["STATUS", "LATENCY", "TOKENS"] : ["", "STATUS", "LATENCY", "TOKENS", "EXTERNAL ID"]).map((h) => (
                    <span
                      key={h || "spacer"}
                      style={{
                        fontFamily: S.mono,
                        fontSize: traceHeaderFontSize,
                        letterSpacing: "0.07em",
                        color: C.faint,
                        textAlign: h === "STATUS" ? "left" : "center",
                        minWidth: 0,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {h}
                    </span>
                  ))}
                </div>
            <div>
              {tracesLoading && (
                <div style={{ minHeight: tracesEmptyMinHeight, display: "flex", flexDirection: "column", justifyContent: "center" }}>
                  {Array.from({ length: 6 }).map((_, idx) => (
                    <div key={idx} style={{ borderBottom: idx < 5 ? `1px solid ${C.border}` : "none" }}>
                      <div style={{ display: "grid", gridTemplateColumns: traceGridColumns, gap: traceGridGap, alignItems: "center", padding: traceRowPadding }}>
                        <Ghost width="64%" height={12} />
                        <Ghost width="80%" height={16} radius={12} />
                        <Ghost width="70%" height={12} />
                        <Ghost width="66%" height={12} />
                        {!isCompact ? <Ghost width="74%" height={12} /> : null}
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {!tracesLoading && traces.length === 0 && false && (
                <div style={{ minHeight: tracesEmptyMinHeight, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textAlign: "center" }}>
                  <div style={{ width: 40, height: 40, borderRadius: "50%", background: C.bgDeep, display: "flex", alignItems: "center", justifyContent: "center", marginBottom: 14 }}>
                    <Activity size={I.lg} color={C.stone} />
                  </div>
                  <p style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 600, color: C.text, margin: "0 0 6px" }}>Monitoring just started</p>
                  <p style={{ fontFamily: S.body, fontSize: T.body, color: C.faint, margin: 0 }}>
                    Trace data will appear here after the first request
                  </p>
                </div>
              )}
              {traces.length > 0 && visibleTraces.length === 0 && (
                <div style={{ padding: "24px 16px", textAlign: "center" }}>
                  <p style={{ fontFamily: S.body, fontSize: T.body, color: C.success, margin: 0 }}>✓ All clear — no errors in this window</p>
                </div>
              )}
              {visibleTraceRows.map((trace) => {
                const st = TRACE_STATUS_STYLE[trace.status];
                const externalId = middleEllipsis(trace.id);
                const copied = copiedTraceId === trace.id;
                return (
                  <div key={trace.id} style={{ borderBottom: `1px solid ${C.border}` }}>
                    <div
                      onClick={() => { onSelectTrace?.(trace); }}
                      style={{ display: "grid", gridTemplateColumns: traceGridColumns, gap: traceGridGap, padding: traceRowPadding, cursor: "pointer", alignItems: "center", transition: "background 0.1s", background: selectedTraceId === trace.id ? C.bgDeep : "transparent" }}
                      onMouseEnter={(e) => {
                        (e.currentTarget as HTMLElement).style.background = C.bgDeep;
                      }}
                      onMouseLeave={(e) => {
                        (e.currentTarget as HTMLElement).style.background = selectedTraceId === trace.id ? C.bgDeep : "transparent";
                      }}
                    >
                      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                        <span style={{ fontFamily: S.mono, fontSize: traceCellFontSize, color: C.text, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{trace.time}</span>
                      </div>
                      {!isCompact ? <span /> : null}
                      <div style={{ width: "100%", display: "flex", justifyContent: "flex-start" }}>
                        <InlineBadge variant="soft" style={st.badgeStyle}>
                          {st.label}
                        </InlineBadge>
                      </div>
                      <div style={{ width: "100%", display: "flex", justifyContent: "center" }}>
                        <span style={{ fontFamily: S.mono, fontSize: traceCellFontSize, color: C.text }}>
                          {formatLatencyMs(trace.latency)}
                        </span>
                      </div>
                      <div style={{ width: "100%", display: "flex", justifyContent: "center" }}>
                        <span style={{ fontFamily: S.mono, fontSize: traceCellFontSize, color: C.text }}>
                          {trace.tokens > 0 ? trace.tokens.toLocaleString() : "—"}
                        </span>
                      </div>
                      {!isCompact ? (
                        <div style={{ width: "100%", display: "grid", gridTemplateColumns: "1fr auto", alignItems: "center", gap: 6 }}>
                          <span
                            style={{
                              fontFamily: S.mono,
                              fontSize: traceCellFontSize,
                              color: C.text,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                          >
                            {externalId}
                          </span>
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              copyTraceId(trace.id);
                            }}
                            style={{
                              background: "none",
                              border: "none",
                              padding: 2,
                              display: "flex",
                              color: C.muted,
                              cursor: "pointer",
                              justifySelf: "end",
                            }}
                            aria-label={`Copy external id ${externalId}`}
                          >
                            {copied ? <Check size={I.sm} /> : <Copy size={I.sm} />}
                          </button>
                        </div>
                      ) : null}
                    </div>
                    {/* inline expansion removed — input/output now in TraceDetailPanel */}
                  </div>
                );
              })}
              {hasCollapsedTraces && (
                <div style={{ display: "flex", justifyContent: "center", padding: "8px 12px 2px" }}>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="font-medium"
                    onClick={() => setShowAllTraces((prev) => !prev)}
                  >
                    {showAllTraces ? (
                      <>Show less <ChevronUpIcon className="h-3.5 w-3.5" /></>
                    ) : (
                      <>Show {visibleTraces.length - 4} more <ChevronDownIcon className="h-3.5 w-3.5" /></>
                    )}
                  </Button>
                </div>
              )}
            </div>
              </div>
            </div>
          </div>
      </div>
    </div>
  );
}
