import { useMemo, useState } from "react";
import { ResponsiveContainer, ComposedChart, Area, Line, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";
import { Activity, Search, ChevronRight } from "lucide-react";
import { mapDeploymentStatus } from "@/lib/deployment-utils";
import { useObservabilityMetrics, useObservabilitySummary, useObservabilityTraces } from "@/api/queries/observability";
import type { AgentDeployment } from "@/lib/api";
import { MultiSelect } from "../shared/MultiSelect";
import { C, S } from "../theme";

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
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}k`;
  return String(n);
}

const CHART_H = 130;

interface ChartTooltipProps {
  active?: boolean;
  payload?: { name: string; value: number; color: string }[];
  label?: string;
  reqVisible: boolean;
  avgLatVisible: boolean;
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
      <div style={{ fontFamily: S.mono, fontSize: 9, color: C.faint, marginBottom: 4, letterSpacing: "0.06em" }}>
        {label}
      </div>
      {reqVisible && req && (
        <div style={{ display: "flex", alignItems: "center", gap: 5, fontFamily: S.mono, fontSize: 11 }}>
          <span style={{ width: 6, height: 6, borderRadius: 1, background: C.tealMid, display: "inline-block", flexShrink: 0 }} />
          <span style={{ color: C.tealMid, fontWeight: 600 }}>{req.value}</span>
          <span style={{ color: C.faint }}>req</span>
        </div>
      )}
      {avgLatVisible && avgLat && (
        <div style={{ display: "flex", alignItems: "center", gap: 5, fontFamily: S.mono, fontSize: 11, marginTop: 2 }}>
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
}: {
  data: { t: string; req: number; avgLatencyMs: number }[];
  reqVisible: boolean;
  avgLatVisible: boolean;
}) {
  if (data.length < 2) return null;
  const reqMax = Math.max(...data.map((d) => d.req));
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
        <XAxis dataKey="t" tick={{ fontFamily: S.mono, fontSize: 8, fill: C.faint }} tickLine={false} axisLine={false} />
        <YAxis
          yAxisId="req"
          orientation="left"
          domain={[0, Math.ceil(reqMax * 1.15)]}
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
            dot={false}
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
            dot={false}
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

function buildTimeParams(win: string) {
  const hours = WIN_HOURS[win] ?? 24;
  const end = new Date();
  const start = new Date(end.getTime() - hours * 60 * 60 * 1000);
  return { start_time: start.toISOString(), end_time: end.toISOString() };
}

export function MonitorTab({ deployment, account }: { deployment: AgentDeployment; account: string }) {
  const [win, setWin] = useState<"1h" | "24h" | "7d">("24h");
  const [traceSearch, setTraceSearch] = useState("");
  const [traceStatuses, setTraceStatuses] = useState<string[]>([]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [series, setSeries] = useState({ req: true, avgLat: true });
  const [tokenView, setTokenView] = useState<"input" | "output">("input");

  const timeParams = buildTimeParams(win);

  const metricsQuery = useObservabilityMetrics(account, deployment.name, timeParams);
  const summaryQuery = useObservabilitySummary(account, deployment.name, timeParams);
  const tracesQuery = useObservabilityTraces(account, deployment.name, { ...timeParams, limit: "100" });
  const { data: metricsData } = metricsQuery;
  const { data: summaryData } = summaryQuery;
  const { data: tracesData } = tracesQuery;
  const observabilityBackendError = metricsQuery.isError || summaryQuery.isError || tracesQuery.isError;

  const tsData: { t: string; req: number; avgLatencyMs: number }[] = (metricsData?.buckets ?? []).map((b) => ({
    t: new Date(b.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    req: b.trace_count,
    avgLatencyMs: b.avg_latency_ms,
  }));

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
    tokens: 0,
    input: t.input,
    output: t.output,
  }));

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

  const summary = summaryData?.metrics;
  const tokenSplitFromBuckets = bucketTokenTotals.sum > 0;
  const tokens = {
    input: tokenSplitFromBuckets ? bucketTokenTotals.input : 0,
    output: tokenSplitFromBuckets ? bucketTokenTotals.output : 0,
    total: summary?.total_tokens ?? bucketTokenTotals.sum,
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
          <span style={{ fontFamily: S.mono, fontSize: 10, fontWeight: 700, letterSpacing: "0.08em", color: C.coral }}>
            ERROR
          </span>
          <span style={{ fontFamily: S.body, fontSize: 12, color: C.coral, flex: 1 }}>
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
          <span style={{ fontFamily: S.mono, fontSize: 10, fontWeight: 700, letterSpacing: "0.08em", color: C.amber }}>
            OBSERVABILITY
          </span>
          <span style={{ fontFamily: S.body, fontSize: 12, color: C.muted, flex: 1, lineHeight: 1.5 }}>
            Trace metrics couldn&apos;t be loaded (backend returned an error). Local dev often needs valid Galileo
            credentials in <span style={{ fontFamily: S.mono, fontSize: 11 }}>astro-server</span> env. Pod logs on
            the <strong style={{ color: C.text }}>Deployments</strong> tab use Kubernetes/Loki and work independently
            when the cluster is reachable.
          </span>
        </div>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "1fr minmax(0, 900px) 1fr", gap: 12, alignItems: "start" }}>
        <div />
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <span style={{ fontFamily: S.body, fontSize: 17, fontWeight: 600, color: C.teal }}>Monitor</span>
            <select
              value={win}
              onChange={(e) => setWin(e.target.value as typeof win)}
              style={{
                padding: "6px 28px 6px 12px",
                borderRadius: 7,
                border: `1px solid ${C.border}`,
                background: C.bg,
                fontFamily: S.body,
                fontSize: 12,
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

          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10 }}>
            {[
              { label: "TOTAL REQUESTS", value: summaryData ? String(summaryData.total_traces) : "—" },
              { label: "ERROR RATE", value: summary ? `${(summary.error_rate * 100).toFixed(1)}%` : "—" },
              { label: "AVG LATENCY", value: summary ? `${summary.avg_latency_ms.toFixed(0)}ms` : "—" },
              { label: "P95 LATENCY", value: summary ? `${summary.p95_latency_ms.toFixed(0)}ms` : "—" },
            ].map(({ label, value }) => (
              <div key={label} style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "12px 14px" }}>
                <span style={{ display: "block", fontFamily: S.mono, fontSize: 9, letterSpacing: "0.07em", color: C.faint, marginBottom: 8 }}>
                  {label}
                </span>
                <span style={{ display: "block", fontFamily: S.body, fontSize: 20, fontWeight: 700, color: C.teal }}>
                  {value}
                </span>
              </div>
            ))}
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, padding: "14px 16px" }}>
              <div style={{ marginBottom: 8 }}>
                <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 700, color: C.teal }}>Request volume</span>
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
                    <span style={{ fontFamily: S.mono, fontSize: 9, color: C.faint }}>{s.label}</span>
                  </button>
                ))}
              </div>
              {tsData.length === 0 ? (
                <div style={{ height: 130, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 10, textAlign: "center" }}>
                  <div style={{ width: 32, height: 32, borderRadius: "50%", background: C.bgDeep, display: "flex", alignItems: "center", justifyContent: "center" }}>
                    <Activity size={15} color={C.stone} />
                  </div>
                  <div>
                    <p style={{ fontFamily: S.body, fontSize: 12, fontWeight: 600, color: C.text, margin: "0 0 3px" }}>No requests yet</p>
                    <p style={{ fontFamily: S.mono, fontSize: 10, color: C.faint, margin: 0, letterSpacing: "0.03em" }}>
                      Volume will appear once traffic starts
                    </p>
                  </div>
                </div>
              ) : (
                <InlineChart key={win} data={tsData} reqVisible={series.req} avgLatVisible={series.avgLat} />
              )}
            </div>

            <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "hidden" }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "12px 16px", borderBottom: `1px solid ${C.border}` }}>
                <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 700, color: C.teal }}>Token usage</span>
                {tokens.hasSplit ? (
                  <div style={{ display: "flex", background: C.bgDeep, borderRadius: 6, padding: 2 }}>
                    {(["input", "output"] as const).map((v) => (
                      <button
                        key={v}
                        type="button"
                        onClick={() => setTokenView(v)}
                        style={{
                          padding: "3px 10px",
                          borderRadius: 4,
                          border: "none",
                          cursor: "pointer",
                          background: tokenView === v ? C.panel : "transparent",
                          fontFamily: S.body,
                          fontSize: 12,
                          color: tokenView === v ? C.text : C.faint,
                          fontWeight: tokenView === v ? 600 : 400,
                          boxShadow: tokenView === v ? "0 1px 3px rgba(0,0,0,0.08)" : "none",
                          textTransform: "capitalize" as const,
                          transition: "all 0.12s",
                        }}
                      >
                        {v}
                      </button>
                    ))}
                  </div>
                ) : (
                  <span style={{ fontFamily: S.mono, fontSize: 10, color: C.faint }}>from summary</span>
                )}
              </div>
              <div style={{ padding: "14px 16px 12px" }}>
                <div style={{ overflow: "hidden", lineHeight: 1.2 }}>
                  <span
                    key={`${win}-${tokenView}-${tokens.hasSplit}`}
                    className="dp-slot-in"
                    style={{
                      display: "block",
                      fontFamily: S.body,
                      fontSize: 26,
                      fontWeight: 700,
                      color: activeToken.color,
                      letterSpacing: "-0.02em",
                    }}
                  >
                    {fmtTokens(activeToken.value)}
                  </span>
                </div>
                {tokens.hasSplit ? (
                  <>
                    <div style={{ display: "flex", height: 5, borderRadius: 3, overflow: "hidden", margin: "12px 0 8px", background: C.bgDeep }}>
                      <div style={{ flex: tokens.input || 1, background: C.tealMid, opacity: tokenView === "input" ? (tokens.total === 0 ? 0.15 : 1) : 0.25, transition: "opacity 0.2s" }} />
                      <div style={{ flex: tokens.output || 1, background: C.amber, opacity: tokenView === "output" ? (tokens.total === 0 ? 0.15 : 1) : 0.25, transition: "opacity 0.2s" }} />
                    </div>
                    <div style={{ display: "flex", justifyContent: "space-between" }}>
                      <div style={{ overflow: "hidden", lineHeight: 1.3 }}>
                        <span key={win} className="dp-slot-in" style={{ display: "block", fontFamily: S.body, fontSize: 11, color: C.faint }}>
                          of {fmtTokens(tokens.total)} total
                        </span>
                      </div>
                      {tokens.total > 0 && (
                        <div style={{ overflow: "hidden", lineHeight: 1.3 }}>
                          <span key={`${win}-pct`} className="dp-slot-in" style={{ display: "block", fontFamily: S.mono, fontSize: 10, color: C.faint }}>
                            {Math.round((activeToken.value / tokens.total) * 100)}%
                          </span>
                        </div>
                      )}
                    </div>
                  </>
                ) : (
                  <p style={{ fontFamily: S.mono, fontSize: 10, color: C.faint, margin: "10px 0 0", lineHeight: 1.5 }}>
                    Input/output split comes from metrics buckets when available.
                  </p>
                )}
              </div>
            </div>
          </div>

          <div style={{ background: C.bgAlt, border: `1px solid ${C.border}`, borderRadius: 10, overflow: "hidden", display: "flex", flexDirection: "column", height: 360 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "12px 16px", borderBottom: `1px solid ${C.border}`, flexShrink: 0 }}>
              <span style={{ fontFamily: S.body, fontSize: 13, fontWeight: 700, color: C.teal, flex: 1 }}>Traces</span>
              <div style={{ display: "flex", alignItems: "center", gap: 5, padding: "4px 8px", borderRadius: 6, border: `1px solid ${C.border}`, background: C.bg }}>
                <Search size={11} color={C.faint} />
                <input
                  type="text"
                  placeholder="Search traces"
                  value={traceSearch}
                  onChange={(e) => setTraceSearch(e.target.value)}
                  style={{ background: "none", border: "none", outline: "none", fontFamily: S.body, fontSize: 11, color: C.muted, width: 160, caretColor: C.tealMid }}
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
                <span key={h} style={{ fontFamily: S.mono, fontSize: 9, letterSpacing: "0.07em", color: C.faint }}>
                  {h}
                </span>
              ))}
            </div>
            <div className="dp-scroll" style={{ flex: 1, overflowY: "auto", minHeight: 0 }}>
              {traces.length === 0 && (
                <div style={{ flex: 1, minHeight: 300, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textAlign: "center" }}>
                  <div style={{ width: 40, height: 40, borderRadius: "50%", background: C.bgDeep, display: "flex", alignItems: "center", justifyContent: "center", marginBottom: 14 }}>
                    <Activity size={18} color={C.stone} />
                  </div>
                  <p style={{ fontFamily: S.body, fontSize: 13, fontWeight: 600, color: C.text, margin: "0 0 6px" }}>Monitoring just started</p>
                  <p style={{ fontFamily: S.mono, fontSize: 11, color: C.faint, margin: 0, letterSpacing: "0.03em" }}>
                    Traces will appear here on first request
                  </p>
                </div>
              )}
              {traces.length > 0 && visibleTraces.length === 0 && (
                <div style={{ padding: "24px 16px", textAlign: "center" }}>
                  <p style={{ fontFamily: S.body, fontSize: 12, color: C.success, margin: 0 }}>✓ All clear — no errors in this window</p>
                </div>
              )}
              {visibleTraces.map((trace) => {
                const st = TRACE_STATUS_STYLE[trace.status];
                const isOpen = expanded.has(trace.id);
                const fmtK = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(0)}k` : String(n));
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
                      <ChevronRight size={12} color={C.faint} style={{ transition: "transform 0.15s", transform: isOpen ? "rotate(90deg)" : "none" }} />
                      <div style={{ minWidth: 0 }}>
                        <div style={{ fontFamily: S.body, fontSize: 12, color: C.text, whiteSpace: "nowrap" as const, overflow: "hidden", textOverflow: "ellipsis" }}>{trace.name}</div>
                        <span style={{ fontFamily: S.mono, fontSize: 9, color: C.faint }}>{trace.id}</span>
                      </div>
                      <span style={{ fontFamily: S.mono, fontSize: 9, padding: "3px 7px", borderRadius: 20, background: st.bg, color: st.color, letterSpacing: "0.05em", justifySelf: "start" as const }}>
                        {st.label}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: 12, color: trace.latency > 2000 ? C.coral : C.muted }}>
                        {trace.latency >= 1000 ? `${(trace.latency / 1000).toFixed(1)}s` : `${trace.latency}ms`}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: 11, color: trace.tokens > 0 ? C.muted : C.faint }}>
                        {trace.tokens > 0 ? fmtK(trace.tokens) : "—"}
                      </span>
                      <span style={{ fontFamily: S.mono, fontSize: 11, color: C.faint }}>{trace.time}</span>
                    </div>
                    {isOpen && (
                      <div style={{ background: C.panel, borderTop: `1px solid ${C.border}` }}>
                        <div style={{ padding: "10px 16px 11px", borderBottom: `1px solid ${C.border}` }}>
                          <span style={{ display: "block", fontFamily: S.mono, fontSize: 9, letterSpacing: "0.09em", color: C.faint, marginBottom: 5 }}>INPUT</span>
                          <span style={{ fontFamily: S.mono, fontSize: 11, color: C.muted, lineHeight: 1.6 }}>{trace.input ?? "—"}</span>
                        </div>
                        <div style={{ padding: "10px 16px 12px" }}>
                          <span style={{ display: "block", fontFamily: S.mono, fontSize: 9, letterSpacing: "0.09em", color: C.faint, marginBottom: 5 }}>OUTPUT</span>
                          {trace.output ? (
                            <span style={{ fontFamily: S.mono, fontSize: 11, color: C.muted, lineHeight: 1.6 }}>{trace.output}</span>
                          ) : (
                            <span style={{ fontFamily: S.mono, fontSize: 11, color: C.coral }}>
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
