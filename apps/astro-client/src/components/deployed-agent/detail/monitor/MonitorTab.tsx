import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ResponsiveContainer, ComposedChart, Area, Line, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { Activity, Search, ChevronRight } from "lucide-react";
import { mapDeploymentStatus } from "@/lib/deployment-utils";
import { useObservabilityMetrics, useObservabilitySummary, useObservabilityTraces } from "@/api/queries/observability";
import { observabilityKeys } from "@/api/queries/keys";
import { api } from "@/lib/api";
import type { AgentDeployment } from "@/lib/api";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { MultiSelect } from "../shared/MultiSelect";
import { HeadlineMetrics, type WindowTrend } from "./HeadlineMetrics";
import { buildPreviousWindowParams, percentChange } from "./trend-utils";

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

type TraceStatus = "success" | "error" | "timeout";

interface TraceRow {
  id: string;
  name: string;
  status: TraceStatus;
  latency: number;
  time: string;
  tokens: number;
  input?: string;
  output?: string;
}

const TRACE_STATUS_STYLE: Record<TraceStatus, { bg: string; color: string; label: string }> = {
  success: { bg: "rgba(45,122,79,0.1)", color: C.success, label: "success" },
  error: { bg: C.coralBg, color: C.coral, label: "error" },
  timeout: { bg: C.amberBg, color: C.amber, label: "timeout" },
};

function fmtTokens(n: number) {
  return Math.round(n).toLocaleString();
}

const CHART_H = 130;

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
      <div style={{ fontFamily: S.mono, fontSize: T.label, color: C.faint, marginBottom: 4, letterSpacing: "0.06em" }}>
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
          <span style={{ width: 8, height: 1.5, background: C.amber, display: "inline-block", flexShrink: 0 }} />
          <span style={{ color: C.amber, fontWeight: 600 }}>{avgLat.value}ms</span>
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
}: {
  data: { t: string; req: number; avgLatencyMs: number }[];
  reqVisible: boolean;
  avgLatVisible: boolean;
  win: "1h" | "24h" | "7d";
}) {
  if (data.length === 0) return null;
  const hasSinglePoint = data.length === 1;
  const reqMax = Math.max(...data.map((d) => d.req));
  const reqUpper = Math.max(1, Math.ceil(reqMax * 1.15));
  const latMin = Math.min(...data.map((d) => d.avgLatencyMs));
  const latMax = Math.max(...data.map((d) => d.avgLatencyMs));
  const latPad = (latMax - latMin) * 0.15 || 50;
  return (
    <ResponsiveContainer width="100%" height={CHART_H}>
      <ComposedChart data={data} margin={{ top: 8, right: 0, bottom: 0, left: 0 }}>
        <defs>
          <linearGradient id="req-grad-obs" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor={C.tealMid} stopOpacity="0.18" />
            <stop offset="95%" stopColor={C.tealMid} stopOpacity="0.02" />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke={C.border} strokeOpacity={0.6} vertical={false} />
        <XAxis
          dataKey="t"
          tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }}
          tickLine={false}
          axisLine={false}
          interval={0}
          minTickGap={0}
          tickMargin={6}
          tickFormatter={(value, index) => {
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
          tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }}
          tickLine={false}
          axisLine={false}
          width={32}
          tickFormatter={(v: number) => (v >= 1000 ? `${(v / 1000).toFixed(0)}k` : String(v))}
          tickCount={4}
        />
        <YAxis
          yAxisId="ms"
          orientation="right"
          domain={[Math.floor(latMin - latPad), Math.ceil(latMax + latPad)]}
          tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }}
          tickLine={false}
          axisLine={false}
          width={38}
          tickFormatter={(v: number) => `${v}ms`}
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
            animationDuration={1000}
            animationEasing="ease-out"
          />
        )}
        {avgLatVisible && (
          <Line
            yAxisId="ms"
            dataKey="avgLatencyMs"
            name="avgLat"
            stroke={C.amber}
            strokeWidth={1.5}
            strokeDasharray="4 3"
            strokeOpacity={0.85}
            dot={hasSinglePoint ? { r: 3.5, fill: C.amber, stroke: C.panel, strokeWidth: 1.5 } : false}
            activeDot={{ r: 4, fill: C.amber, stroke: C.panel, strokeWidth: 1.5 }}
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

export function MonitorTab({ deployment, account }: { deployment: AgentDeployment; account: string }) {
  const queryClient = useQueryClient();
  const [win, setWin] = useState<Win>("24h");
  const [traceSearch, setTraceSearch] = useState("");
  const [traceStatuses, setTraceStatuses] = useState<string[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [series, setSeries] = useState({ req: true, avgLat: true });
  const [tokenView, setTokenView] = useState<"input" | "output">("input");
  const [windowParams] = useState<Record<Win, { start_time: string; end_time: string }>>(() => ({
    "1h": buildTimeParams("1h"),
    "24h": buildTimeParams("24h"),
    "7d": buildTimeParams("7d"),
  }));
  const timeParams = windowParams[win];
  const previousWindowParams = useMemo(() => buildPreviousWindowParams(windowParams), [windowParams]);

  useEffect(() => {
    if (!account || !deployment.name) return;

    for (const window of OBS_WINDOWS) {
      const params = windowParams[window];
      const prevParams = previousWindowParams[window];
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.metrics(account, deployment.name, params),
        queryFn: () => api.getObservabilityMetrics(account, deployment.name, params),
      });
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.summary(account, deployment.name, params),
        queryFn: () => api.getObservabilitySummary(account, deployment.name, params),
      });
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.traces(account, deployment.name, { ...params, limit: "100" }),
        queryFn: () => api.getObservabilityTraces(account, deployment.name, { ...params, limit: "100" }),
      });
      void queryClient.prefetchQuery({
        queryKey: observabilityKeys.summary(account, deployment.name, prevParams),
        queryFn: () => api.getObservabilitySummary(account, deployment.name, prevParams),
      });
    }
  }, [account, deployment.name, queryClient, windowParams, previousWindowParams]);

  const metricsQuery = useObservabilityMetrics(account, deployment.name, timeParams);
  const tracesQuery = useObservabilityTraces(account, deployment.name, { ...timeParams, limit: "100" });
  const summary1hQuery = useObservabilitySummary(account, deployment.name, windowParams["1h"]);
  const summary24hQuery = useObservabilitySummary(account, deployment.name, windowParams["24h"]);
  const summary7dQuery = useObservabilitySummary(account, deployment.name, windowParams["7d"]);
  const prevSummary1hQuery = useObservabilitySummary(account, deployment.name, previousWindowParams["1h"]);
  const prevSummary24hQuery = useObservabilitySummary(account, deployment.name, previousWindowParams["24h"]);
  const prevSummary7dQuery = useObservabilitySummary(account, deployment.name, previousWindowParams["7d"]);

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
    time: new Date(t.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    tokens: t.total_tokens ?? 0,
    input: t.input,
    output: t.output,
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

  const visibleTraces = traces.filter((t) => {
    if (traceStatuses.length > 0 && !traceStatuses.includes(t.status)) return false;
    if (traceSearch && !t.name.toLowerCase().includes(traceSearch.toLowerCase())) return false;
    return true;
  });

  const toggleTrace = (id: string) =>
    setExpanded((prev) => {
      const n = new Set(prev);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

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
      ? { value: tokens.total, color: C.tealMid }
      : tokenView === "input"
        ? { value: tokens.input, color: C.tealMid }
        : { value: tokens.output, color: C.amber };

  const isError = mapDeploymentStatus(deployment) === "error";

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

      {observabilityBackendError && !isError && (
        <div
          style={{
            display: "flex",
            alignItems: "flex-start",
            gap: 10,
            padding: "10px 16px",
            borderRadius: 8,
            background: C.amberBg,
            border: `1px solid ${C.amberBdr}`,
          }}
        >
          <span style={{ fontFamily: S.mono, fontSize: T.label, fontWeight: 700, letterSpacing: "0.08em", color: C.amber }}>
            OBSERVABILITY
          </span>
          <span style={{ fontFamily: S.body, fontSize: T.body, color: C.muted, flex: 1, lineHeight: 1.5 }}>
            Trace metrics couldn&apos;t be loaded (backend returned an error). Local dev often needs valid Galileo
            credentials in <span style={{ fontFamily: S.mono, fontSize: T.monoSm }}>astro-server</span> env. Pod logs on
            the <strong style={{ color: C.text }}>Deployments</strong> tab use Kubernetes/Loki and work independently
            when the cluster is reachable.
          </span>
        </div>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "1fr minmax(0, 900px) 1fr", gap: 12, alignItems: "start" }}>
        <div />
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <span style={{ fontFamily: S.body, fontSize: T.heading2, fontWeight: 600, color: C.teal }}>Monitor</span>
            <select
              value={win}
              onChange={(e) => setWin(e.target.value as typeof win)}
              style={{
                padding: "6px 28px 6px 12px",
                borderRadius: 7,
                border: `1px solid ${C.border}`,
                background: C.bg,
                fontFamily: S.body,
                fontSize: T.body,
                color: C.muted,
                cursor: "pointer",
                outline: "none",
                appearance: "none" as const,
                backgroundImage:
                  "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10' viewBox='0 0 24 24' fill='none' stroke='%236b7e7c' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpolyline points='6 9 12 15 18 9'%3E%3C/polyline%3E%3C/svg%3E\")",
                backgroundRepeat: "no-repeat",
                backgroundPosition: "right 10px center",
              }}
            >
              <option value="1h">Last 1 hour</option>
              <option value="24h">Last 24 hours</option>
              <option value="7d">Last 7 days</option>
            </select>
          </div>

          <HeadlineMetrics
            summary={summaryData ? { total_traces: summaryData.total_traces, metrics: summaryData.metrics } : null}
            summaryLoading={summaryLoading}
            trendLoading={trendLoading}
            selectedWindow={win}
            trends={trends}
          />

          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ marginBottom: 8 }}>
                <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 700, color: C.teal }}>Request volume</span>
              </div>
              <div style={{ display: "flex", gap: 10, marginBottom: 6 }}>
                {[
                  { key: "req" as const, color: C.tealMid, label: "Requests", dashed: false },
                  { key: "avgLat" as const, color: C.amber, label: "Avg latency", dashed: true },
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
                    <span style={{ fontFamily: S.mono, fontSize: T.label, color: C.faint }}>{s.label}</span>
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
                    <p style={{ fontFamily: S.mono, fontSize: T.label, color: C.faint, margin: 0, letterSpacing: "0.03em" }}>
                      Volume will appear once traffic starts
                    </p>
                  </div>
                </div>
              ) : (
                <InlineChart key={win} data={tsData} reqVisible={series.req} avgLatVisible={series.avgLat} win={win} />
              )}
            </div>

            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 14, overflow: "hidden" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 18px", borderBottom: `1px solid ${C.border}` }}>
                <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 700, color: C.teal }}>Token usage</span>
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
                      <div style={{ flex: tokens.output || 1, background: C.amber, opacity: tokenView === "output" ? (tokens.total === 0 ? 0.2 : 1) : 0.35, transition: "opacity 0.2s" }} />
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

          <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "hidden", display: "flex", flexDirection: "column", height: 420 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "12px 16px", borderBottom: `1px solid ${C.border}`, flexShrink: 0 }}>
              <span style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 700, color: C.teal, flex: 1 }}>Traces</span>
              <div style={{ display: "flex", alignItems: "center", gap: 5, padding: "4px 8px", borderRadius: 6, border: `1px solid ${C.border}`, background: C.bg }}>
                <Search size={I.sm} color={C.faint} />
                <input
                  type="text"
                  placeholder="Search traces"
                  value={traceSearch}
                  onChange={(e) => setTraceSearch(e.target.value)}
                  style={{ background: "none", border: "none", outline: "none", fontFamily: S.body, fontSize: T.bodySm, color: C.muted, width: 160, caretColor: C.tealMid }}
                />
              </div>
              <MultiSelect
                options={[
                  { value: "success", label: "Success", color: C.success },
                  { value: "error", label: "Error", color: C.coral },
                  { value: "timeout", label: "Timeout", color: C.amber },
                ]}
                selected={traceStatuses}
                onChange={setTraceStatuses}
                placeholder="All statuses"
              />
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "16px 1fr 80px 72px 60px 72px", gap: 10, padding: "7px 16px", borderBottom: `1px solid ${C.border}`, background: C.bgDeep, flexShrink: 0 }}>
              {["", "TRACE", "STATUS", "LATENCY", "TOKENS", "TIME"].map((h) => (
                <span key={h} style={{ fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.07em", color: C.faint }}>
                  {h}
                </span>
              ))}
            </div>
            <div className="dp-scroll" style={{ flex: 1, overflowY: "auto", minHeight: 0, paddingBottom: 12 }}>
              {tracesLoading && (
                <div style={{ flex: 1, minHeight: 300, display: "flex", flexDirection: "column", justifyContent: "center" }}>
                  {Array.from({ length: 7 }).map((_, idx) => (
                    <div key={idx} style={{ borderBottom: idx < 6 ? `1px solid ${C.border}` : "none" }}>
                      <div style={{ display: "grid", gridTemplateColumns: "16px 1fr 80px 72px 60px 72px", gap: 10, alignItems: "center", padding: "10px 16px" }}>
                        <Ghost width={12} height={12} radius={4} />
                        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                          <Ghost width="78%" height={12} />
                          <Ghost width="42%" height={9} />
                        </div>
                        <Ghost width="80%" height={16} radius={12} />
                        <Ghost width="90%" height={12} />
                        <Ghost width="80%" height={12} />
                        <Ghost width="85%" height={12} />
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {!tracesLoading && traces.length === 0 && (
                <div style={{ flex: 1, minHeight: 300, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textAlign: "center" }}>
                  <div style={{ width: 40, height: 40, borderRadius: "50%", background: C.bgDeep, display: "flex", alignItems: "center", justifyContent: "center", marginBottom: 14 }}>
                    <Activity size={I.lg} color={C.stone} />
                  </div>
                  <p style={{ fontFamily: S.body, fontSize: T.heading4, fontWeight: 600, color: C.text, margin: "0 0 6px" }}>Monitoring just started</p>
                  <p style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint, margin: 0, letterSpacing: "0.03em" }}>
                    Traces will appear here on first request
                  </p>
                </div>
              )}
              {traces.length > 0 && visibleTraces.length === 0 && (
                <div style={{ padding: "24px 16px", textAlign: "center" }}>
                  <p style={{ fontFamily: S.body, fontSize: T.body, color: C.success, margin: 0 }}>✓ All clear — no errors in this window</p>
                </div>
              )}
              {visibleTraces.map((trace) => {
                const st = TRACE_STATUS_STYLE[trace.status];
                const isOpen = expanded.has(trace.id);
                return (
                  <div key={trace.id} style={{ borderBottom: `1px solid ${C.border}` }}>
                    <div
                      onClick={() => toggleTrace(trace.id)}
                      style={{ display: "grid", gridTemplateColumns: "16px 1fr 80px 72px 60px 72px", gap: 10, padding: "10px 16px", cursor: "pointer", alignItems: "center", transition: "background 0.1s" }}
                      onMouseEnter={(e) => {
                        (e.currentTarget as HTMLElement).style.background = C.bgDeep;
                      }}
                      onMouseLeave={(e) => {
                        (e.currentTarget as HTMLElement).style.background = "transparent";
                      }}
                    >
                      <ChevronRight size={I.sm} color={C.faint} style={{ transition: "transform 0.15s", transform: isOpen ? "rotate(90deg)" : "none" }} />
                      <div style={{ minWidth: 0 }}>
                        <div style={{ fontFamily: S.body, fontSize: T.body, color: C.text, whiteSpace: "nowrap" as const, overflow: "hidden", textOverflow: "ellipsis" }}>{trace.name}</div>
                        <span style={{ fontFamily: S.mono, fontSize: T.label, color: C.faint }}>{trace.id}</span>
                      </div>
                      <span style={{ fontFamily: S.mono, fontSize: T.label, padding: "3px 7px", borderRadius: 20, background: st.bg, color: st.color, letterSpacing: "0.05em", justifySelf: "start" as const }}>
                        {st.label}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: T.monoMd, color: trace.latency > 2000 ? C.coral : C.muted }}>
                        {trace.latency >= 1000 ? `${(trace.latency / 1000).toFixed(1)}s` : `${trace.latency}ms`}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: trace.tokens > 0 ? C.muted : C.faint }}>
                        {trace.tokens > 0 ? trace.tokens.toLocaleString() : "—"}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.faint }}>{trace.time}</span>
                    </div>
                    {isOpen && (
                      <div style={{ background: C.panel, borderTop: `1px solid ${C.border}` }}>
                        <div style={{ padding: "10px 16px 11px", borderBottom: `1px solid ${C.border}` }}>
                          <span style={{ display: "block", fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.09em", color: C.faint, marginBottom: 5 }}>INPUT</span>
                          <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.muted, lineHeight: 1.6 }}>{trace.input ?? "—"}</span>
                        </div>
                        <div style={{ padding: "10px 16px 12px" }}>
                          <span style={{ display: "block", fontFamily: S.mono, fontSize: T.label, letterSpacing: "0.09em", color: C.faint, marginBottom: 5 }}>OUTPUT</span>
                          {trace.output ? (
                            <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.muted, lineHeight: 1.6 }}>{trace.output}</span>
                          ) : (
                            <span style={{ fontFamily: S.mono, fontSize: T.monoSm, color: C.coral }}>
                              Trace did not complete — no output recorded
                            </span>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
        <div />
      </div>
    </div>
  );
}
