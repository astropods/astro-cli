import { useMemo, useState } from "react";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  ReferenceLine,
} from "recharts";
import { Info, Loader2 } from "lucide-react";
import { CHART_COLORS } from "@/components/agent-detail/charts/chart-utils";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import {
  Tooltip as HintTooltip,
  TooltipContent as HintTooltipContent,
  TooltipProvider as HintTooltipProvider,
  TooltipTrigger as HintTooltipTrigger,
} from "@/components/ui/tooltip";
import { useResolvedTheme } from "@/lib/theme";
import { usePodMetrics } from "@/api/queries/deployments";
import type { PodMetricPoint, PodMetricsRange, PodMetricsResponse } from "@/lib/api";

const RANGES = [
  { key: "1h", label: "1H" },
  { key: "6h", label: "6H" },
  { key: "24h", label: "24H" },
  { key: "7d", label: "7D" },
] as const;

type MarkerKind = "restart" | "oom";

interface ChartMarker {
  /** Epoch millis — the X-axis coordinate of the marker. */
  t: number;
  kind: MarkerKind;
  /** Human label used in the tooltip / aria-label. */
  label: string;
}

const MARKER_STYLE: Record<MarkerKind, { stroke: string; label: string }> = {
  restart: { stroke: "var(--color-amber-500)", label: "Restart" },
  oom:     { stroke: "var(--color-red-500)",   label: "OOM kill" },
};

/** Collects restart and (optionally) OOM timestamps into a single sorted array
 *  of typed markers ready to feed into ReferenceLine. Unknown timestamps are
 *  filtered out. */
function buildMarkers(input: {
  restarts?: string[] | null;
  ooms?: string[] | null;
}): ChartMarker[] {
  const out: ChartMarker[] = [];
  for (const iso of input.restarts ?? []) {
    const t = new Date(iso).getTime();
    if (Number.isFinite(t)) out.push({ t, kind: "restart", label: "Restart" });
  }
  for (const iso of input.ooms ?? []) {
    const t = new Date(iso).getTime();
    if (Number.isFinite(t)) out.push({ t, kind: "oom", label: "OOM kill" });
  }
  out.sort((a, b) => a.t - b.t);
  return out;
}

interface PodMetricsTabProps {
  deploymentId: string;
  podName: string | undefined;
}

export function PodMetricsTab({ deploymentId, podName }: PodMetricsTabProps) {
  const [range, setRange] = useState<PodMetricsRange>("1h");
  const resolvedTheme = useResolvedTheme();
  const colors = resolvedTheme === "dark" ? CHART_COLORS.dark : CHART_COLORS.light;
  const enabled = !!podName;
  const { data, isLoading, isError, error } = usePodMetrics(deploymentId, podName ?? "", range, enabled);

  if (!podName) {
    return (
      <p className="text-body-sm text-faint-foreground">
        Metrics are only available for running pods.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <p className="text-body-sm text-muted-foreground">
          Resource usage{data?.step ? ` sampled every ${data.step}` : ""}.
        </p>
        <TimeRangeSelector
          value={range}
          ranges={RANGES as unknown as { key: string; label: string }[]}
          onChange={(v) => setRange(v as PodMetricsRange)}
          layoutId="pod-metrics-range"
        />
      </div>

      {isError ? (
        <p className="text-body-sm text-destructive">
          {error instanceof Error ? error.message : "Failed to load metrics"}
        </p>
      ) : null}

      <MetricChart
        title="CPU"
        unit="cores"
        points={data?.cpu ?? []}
        loading={isLoading}
        color={colors.inputFill}
        formatValue={formatCores}
        markers={buildMarkers({ restarts: data?.restarts })}
        hint={
          <span>
            CPU usage in vCPU cores. Values under one core are shown in
            <span className="font-mono"> millicores</span> (suffix{" "}
            <span className="font-mono">m</span>) where{" "}
            <span className="font-mono">1000m = 1 core</span>. So{" "}
            <span className="font-mono">250m</span> is a quarter of a core, and{" "}
            <span className="font-mono">0m</span> means the pod is essentially
            idle (under half a millicore).
          </span>
        }
      />

      <MetricChart
        title="Memory"
        unit="working set"
        points={data?.memory ?? []}
        loading={isLoading}
        color={colors.inputFill}
        formatValue={formatBytes}
        markers={buildMarkers({ restarts: data?.restarts, ooms: data?.ooms })}
        hint={
          <span>
            <span className="font-medium">Working set</span> is roughly the
            memory the pod can&apos;t give back to the kernel without paging — used
            heap, anonymous mappings, and recently-touched file cache. It&apos;s the
            number the kernel checks against the pod&apos;s memory limit, so it&apos;s
            the value most likely to trigger an{" "}
            <span className="font-mono">OOMKilled</span>.
          </span>
        }
      />

      {data && hasStorage(data) && (
        <StorageChart
          used={data.storage_used ?? []}
          capacity={data.storage_capacity ?? []}
          loading={isLoading}
          color={colors.inputFill}
          markers={buildMarkers({ restarts: data?.restarts })}
          hint={
            <span>
              Bytes currently used by the pod&apos;s persistent volumes, summed
              across every claim mounted by the pod. The dashed line on the
              chart marks the provisioned capacity — once the used line meets
              it, writes will start failing. Persistent volumes can be
              expanded but never shrunk.
            </span>
          }
        />
      )}

      <DualLineChart
        title="Network"
        unit="bytes / sec"
        seriesA={{ label: "In", points: data?.network_rx ?? [], color: colors.inputFill }}
        seriesB={{ label: "Out", points: data?.network_tx ?? [], color: colors.outputFill }}
        loading={isLoading}
        formatValue={formatBytesPerSec}
        markers={buildMarkers({ restarts: data?.restarts })}
        hint={
          <span>
            Network throughput across every interface in the pod, measured at
            the pod&apos;s network namespace. <span className="font-medium">In</span>{" "}
            is bytes received per second from any source (other pods, the
            internet, loopback) and <span className="font-medium">Out</span> is
            bytes sent. Counted before any TLS/HTTP overhead, so values are
            slightly higher than application-level request sizes.
          </span>
        }
      />

      <DualLineChart
        title="Filesystem"
        unit="bytes / sec"
        seriesA={{ label: "Read", points: data?.fs_read ?? [], color: colors.inputFill }}
        seriesB={{ label: "Write", points: data?.fs_write ?? [], color: colors.outputFill }}
        loading={isLoading}
        formatValue={formatBytesPerSec}
        markers={buildMarkers({ restarts: data?.restarts })}
        hint={
          <span>
            Disk throughput per second, summed across the pod&apos;s containers
            and across every block device they touch — both persistent volumes
            and the writable layer of the container image. Mostly zero unless
            the agent reads or writes files directly (databases, caches,
            uploads).
          </span>
        }
      />

    </div>
  );
}

function hasStorage(data: PodMetricsResponse): boolean {
  return (data.storage_used?.length ?? 0) > 0;
}

// ---------------------------------------------------------------------------
// Chart card — shared shell for CPU / memory / storage tiles.
// ---------------------------------------------------------------------------

interface ChartCardProps {
  title: string;
  unit: string;
  headline: string;
  /** Optional contextual help shown on hover of an info icon next to the
   *  title — used to explain non-obvious units (e.g. CPU millicores). */
  hint?: React.ReactNode;
  loading?: boolean;
  empty?: boolean;
  children: React.ReactNode;
}

function ChartCard({ title, unit, headline, hint, loading, empty, children }: ChartCardProps) {
  return (
    <div className="rounded-lg border border-border/60 bg-card dark:bg-surface p-5">
      <div className="mb-4 flex items-baseline justify-between gap-3">
        <div>
          <div className="flex items-center gap-1.5">
            <span className="text-heading-4 text-foreground">{title}</span>
            {hint && (
              <HintTooltipProvider delayDuration={150}>
                <HintTooltip>
                  <HintTooltipTrigger asChild>
                    <button
                      type="button"
                      aria-label={`About ${title}`}
                      className="flex items-center justify-center rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground"
                    >
                      <Info className="size-3.5" />
                    </button>
                  </HintTooltipTrigger>
                  <HintTooltipContent side="right" className="max-w-xs whitespace-normal text-left">
                    {hint}
                  </HintTooltipContent>
                </HintTooltip>
              </HintTooltipProvider>
            )}
          </div>
          <div className="text-body-sm text-muted-foreground">{unit}</div>
        </div>
        <div className="text-heading-4 tabular-nums text-foreground">
          {loading ? "—" : headline}
        </div>
      </div>
      <div className="h-48">
        {loading ? (
          <div className="flex h-full items-center justify-center">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : empty ? (
          <div className="flex h-full items-center justify-center text-body-sm text-faint-foreground">
            No data in this range.
          </div>
        ) : (
          children
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Single-series chart used by CPU and memory.
// ---------------------------------------------------------------------------

interface MetricChartProps {
  title: string;
  unit: string;
  points: PodMetricPoint[];
  loading?: boolean;
  color: string;
  formatValue: (value: number) => string;
  markers?: ChartMarker[];
  hint?: React.ReactNode;
}

function MetricChart({ title, unit, points, loading, color, formatValue, markers = [], hint }: MetricChartProps) {
  const ticks = useMemo(() => buildTimeTicks(points), [points]);
  const rows = useMemo(
    () => points.map((p) => ({ t: new Date(p.timestamp).getTime(), value: p.value })),
    [points],
  );
  const latest = rows.length > 0 ? rows[rows.length - 1].value : 0;
  const empty = rows.length === 0;
  const gradId = `metric-grad-${title.toLowerCase()}`;

  return (
    <ChartCard
      title={title}
      unit={unit}
      headline={formatValue(latest)}
      hint={hint}
      loading={loading}
      empty={empty}
    >
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.35} />
              <stop offset="95%" stopColor={color} stopOpacity={0.03} />
            </linearGradient>
          </defs>
          <CartesianGrid
            strokeDasharray="3 3"
            vertical={false}
            stroke="var(--color-border)"
            strokeOpacity={0.5}
          />
          <XAxis
            dataKey="t"
            type="number"
            domain={["dataMin", "dataMax"]}
            scale="time"
            ticks={ticks}
            tickFormatter={formatTimeTick(rangeMs(rows))}
            tick={{
              fill: "var(--color-muted-foreground)",
              fontSize: 11,
              fontFamily: "var(--font-mono)",
            }}
            axisLine={false}
            tickLine={false}
            tickMargin={8}
            minTickGap={20}
          />
          <YAxis
            tickFormatter={formatValue}
            tick={{
              fill: "var(--color-muted-foreground)",
              fontSize: 11,
              fontFamily: "var(--font-mono)",
            }}
            axisLine={false}
            tickLine={false}
            tickMargin={4}
            width={60}
            domain={[0, "auto"]}
          />
          <Tooltip
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null;
              const row = payload[0].payload as { t: number; value: number };
              return (
                <div className="rounded-md border border-border bg-surface/95 px-3 py-2 text-body-sm shadow-lg backdrop-blur supports-[backdrop-filter]:bg-surface/90">
                  <p className="mb-1 text-mono-sm text-muted-foreground">
                    {formatTooltipTimestamp(row.t)}
                  </p>
                  <span className="font-mono font-medium text-foreground">
                    {formatValue(row.value)}
                  </span>
                </div>
              );
            }}
            cursor={{
              stroke: "var(--color-border)",
              strokeWidth: 1,
              strokeDasharray: "4 4",
            }}
          />
          {markers.map((m, i) => (
            <ReferenceLine
              key={`${m.kind}-${m.t}-${i}`}
              x={m.t}
              stroke={MARKER_STYLE[m.kind].stroke}
              strokeWidth={1.5}
              strokeDasharray="3 3"
              ifOverflow="extendDomain"
            />
          ))}
          <Area
            type="monotone"
            dataKey="value"
            fill={`url(#${gradId})`}
            stroke={color}
            strokeWidth={2}
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
      <MarkerLegend markers={markers} />
    </ChartCard>
  );
}

// ---------------------------------------------------------------------------
// Storage chart — used vs capacity with % headline.
// ---------------------------------------------------------------------------

interface StorageChartProps {
  used: PodMetricPoint[];
  capacity: PodMetricPoint[];
  loading?: boolean;
  color: string;
  markers?: ChartMarker[];
  hint?: React.ReactNode;
}

function StorageChart({ used, capacity, loading, color, markers = [], hint }: StorageChartProps) {
  const rows = useMemo(() => {
    const capByTs = new Map<string, number>();
    for (const p of capacity) capByTs.set(p.timestamp, p.value);
    return used.map((p) => ({
      t: new Date(p.timestamp).getTime(),
      used: p.value,
      capacity: capByTs.get(p.timestamp) ?? 0,
    }));
  }, [used, capacity]);
  const ticks = useMemo(() => buildTimeTicks(used), [used]);

  const latest = rows.length > 0 ? rows[rows.length - 1] : { used: 0, capacity: 0 };
  const pctUsed = latest.capacity > 0 ? (latest.used / latest.capacity) * 100 : 0;
  const empty = rows.length === 0;

  return (
    <ChartCard
      title="Storage"
      unit={
        latest.capacity > 0
          ? `${formatBytes(latest.used)} of ${formatBytes(latest.capacity)}`
          : "persistent volume"
      }
      headline={`${pctUsed.toFixed(0)}%`}
      hint={hint}
      loading={loading}
      empty={empty}
    >
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id="storage-grad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.35} />
              <stop offset="95%" stopColor={color} stopOpacity={0.03} />
            </linearGradient>
          </defs>
          <CartesianGrid
            strokeDasharray="3 3"
            vertical={false}
            stroke="var(--color-border)"
            strokeOpacity={0.5}
          />
          <XAxis
            dataKey="t"
            type="number"
            domain={["dataMin", "dataMax"]}
            scale="time"
            ticks={ticks}
            tickFormatter={formatTimeTick(rangeMs(rows))}
            tick={{
              fill: "var(--color-muted-foreground)",
              fontSize: 11,
              fontFamily: "var(--font-mono)",
            }}
            axisLine={false}
            tickLine={false}
            tickMargin={8}
            minTickGap={20}
          />
          <YAxis
            tickFormatter={formatBytes}
            tick={{
              fill: "var(--color-muted-foreground)",
              fontSize: 11,
              fontFamily: "var(--font-mono)",
            }}
            axisLine={false}
            tickLine={false}
            tickMargin={4}
            width={60}
            domain={[0, latest.capacity > 0 ? latest.capacity : "auto"]}
          />
          {latest.capacity > 0 && (
            <ReferenceLine
              y={latest.capacity}
              stroke="var(--color-muted-foreground)"
              strokeDasharray="4 4"
              strokeOpacity={0.6}
            />
          )}
          <Tooltip
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null;
              const row = payload[0].payload as {
                t: number;
                used: number;
                capacity: number;
              };
              return (
                <div className="rounded-md border border-border bg-surface/95 px-3 py-2 text-body-sm shadow-lg backdrop-blur supports-[backdrop-filter]:bg-surface/90">
                  <p className="mb-1 text-mono-sm text-muted-foreground">
                    {formatTooltipTimestamp(row.t)}
                  </p>
                  <div className="flex flex-col gap-0.5 font-mono">
                    <span className="text-foreground">{formatBytes(row.used)} used</span>
                    <span className="text-muted-foreground">
                      {formatBytes(row.capacity)} provisioned
                    </span>
                  </div>
                </div>
              );
            }}
            cursor={{
              stroke: "var(--color-border)",
              strokeWidth: 1,
              strokeDasharray: "4 4",
            }}
          />
          {markers.map((m, i) => (
            <ReferenceLine
              key={`${m.kind}-${m.t}-${i}`}
              x={m.t}
              stroke={MARKER_STYLE[m.kind].stroke}
              strokeWidth={1.5}
              strokeDasharray="3 3"
              ifOverflow="extendDomain"
            />
          ))}
          <Area
            type="monotone"
            dataKey="used"
            fill="url(#storage-grad)"
            stroke={color}
            strokeWidth={2}
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
      <MarkerLegend markers={markers} />
    </ChartCard>
  );
}

// ---------------------------------------------------------------------------
// Dual-series chart — two lines sharing an X axis, used for Network (in/out)
// and Filesystem (read/write). Headline shows the latest combined throughput.
// ---------------------------------------------------------------------------

interface SeriesInput {
  label: string;
  points: PodMetricPoint[];
  color: string;
}

interface DualLineChartProps {
  title: string;
  unit: string;
  seriesA: SeriesInput;
  seriesB: SeriesInput;
  loading?: boolean;
  formatValue: (value: number) => string;
  markers?: ChartMarker[];
  hint?: React.ReactNode;
}

function DualLineChart({
  title,
  unit,
  seriesA,
  seriesB,
  loading,
  formatValue,
  markers = [],
  hint,
}: DualLineChartProps) {
  // Align by timestamp so the two series share an X axis. Missing samples on
  // either side fall through as undefined — recharts draws gaps cleanly.
  const rows = useMemo(() => {
    const bByTs = new Map<string, number>();
    for (const p of seriesB.points) bByTs.set(p.timestamp, p.value);
    const aByTs = new Map<string, number>();
    for (const p of seriesA.points) aByTs.set(p.timestamp, p.value);
    const timestamps = Array.from(new Set([...aByTs.keys(), ...bByTs.keys()])).sort();
    return timestamps.map((ts) => ({
      t: new Date(ts).getTime(),
      a: aByTs.get(ts),
      b: bByTs.get(ts),
    }));
  }, [seriesA.points, seriesB.points]);

  const ticks = useMemo(
    () => buildTimeTicks(seriesA.points.length >= seriesB.points.length ? seriesA.points : seriesB.points),
    [seriesA.points, seriesB.points],
  );

  const latest = rows.length > 0 ? rows[rows.length - 1] : { a: 0, b: 0 };
  const total = (latest.a ?? 0) + (latest.b ?? 0);
  const empty = rows.length === 0;

  return (
    <ChartCard
      title={title}
      unit={unit}
      headline={formatValue(total)}
      hint={hint}
      loading={loading}
      empty={empty}
    >
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={`dual-grad-a-${title}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={seriesA.color} stopOpacity={0.3} />
              <stop offset="95%" stopColor={seriesA.color} stopOpacity={0.03} />
            </linearGradient>
            <linearGradient id={`dual-grad-b-${title}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={seriesB.color} stopOpacity={0.3} />
              <stop offset="95%" stopColor={seriesB.color} stopOpacity={0.03} />
            </linearGradient>
          </defs>
          <CartesianGrid
            strokeDasharray="3 3"
            vertical={false}
            stroke="var(--color-border)"
            strokeOpacity={0.5}
          />
          <XAxis
            dataKey="t"
            type="number"
            domain={["dataMin", "dataMax"]}
            scale="time"
            ticks={ticks}
            tickFormatter={formatTimeTick(rangeMs(rows))}
            tick={{
              fill: "var(--color-muted-foreground)",
              fontSize: 11,
              fontFamily: "var(--font-mono)",
            }}
            axisLine={false}
            tickLine={false}
            tickMargin={8}
            minTickGap={20}
          />
          <YAxis
            tickFormatter={formatValue}
            tick={{
              fill: "var(--color-muted-foreground)",
              fontSize: 11,
              fontFamily: "var(--font-mono)",
            }}
            axisLine={false}
            tickLine={false}
            tickMargin={4}
            width={60}
            domain={[0, "auto"]}
          />
          {markers.map((m, i) => (
            <ReferenceLine
              key={`${m.kind}-${m.t}-${i}`}
              x={m.t}
              stroke={MARKER_STYLE[m.kind].stroke}
              strokeWidth={1.5}
              strokeDasharray="3 3"
              ifOverflow="extendDomain"
            />
          ))}
          <Tooltip
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null;
              const row = payload[0].payload as { t: number; a?: number; b?: number };
              return (
                <div className="rounded-md border border-border bg-surface/95 px-3 py-2 text-body-sm shadow-lg backdrop-blur supports-[backdrop-filter]:bg-surface/90">
                  <p className="mb-1 text-mono-sm text-muted-foreground">
                    {formatTooltipTimestamp(row.t)}
                  </p>
                  <div className="flex flex-col gap-0.5 font-mono">
                    <span className="flex items-center gap-2">
                      <span className="size-2 rounded-full" style={{ backgroundColor: seriesA.color }} />
                      <span className="text-muted-foreground">{seriesA.label}</span>
                      <span className="ml-auto text-foreground">{formatValue(row.a ?? 0)}</span>
                    </span>
                    <span className="flex items-center gap-2">
                      <span className="size-2 rounded-full" style={{ backgroundColor: seriesB.color }} />
                      <span className="text-muted-foreground">{seriesB.label}</span>
                      <span className="ml-auto text-foreground">{formatValue(row.b ?? 0)}</span>
                    </span>
                  </div>
                </div>
              );
            }}
            cursor={{
              stroke: "var(--color-border)",
              strokeWidth: 1,
              strokeDasharray: "4 4",
            }}
          />
          <Area
            type="monotone"
            dataKey="a"
            name={seriesA.label}
            fill={`url(#dual-grad-a-${title})`}
            stroke={seriesA.color}
            strokeWidth={2}
            isAnimationActive={false}
          />
          <Area
            type="monotone"
            dataKey="b"
            name={seriesB.label}
            fill={`url(#dual-grad-b-${title})`}
            stroke={seriesB.color}
            strokeWidth={2}
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
      <div className="mt-3 flex items-center gap-4 text-mono-sm text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <span className="size-2 rounded-full" style={{ backgroundColor: seriesA.color }} />
          {seriesA.label}
        </span>
        <span className="flex items-center gap-1.5">
          <span className="size-2 rounded-full" style={{ backgroundColor: seriesB.color }} />
          {seriesB.label}
        </span>
      </div>
      <MarkerLegend markers={markers} />
    </ChartCard>
  );
}

// ---------------------------------------------------------------------------
// Marker legend — small inline pill row shown under any chart that has at
// least one event in the current window. Counts are summarized per kind so
// the chart stays readable when there are many markers.
// ---------------------------------------------------------------------------

function MarkerLegend({ markers }: { markers: ChartMarker[] }) {
  const counts = useMemo(() => {
    const c: Partial<Record<MarkerKind, number>> = {};
    for (const m of markers) c[m.kind] = (c[m.kind] ?? 0) + 1;
    return c;
  }, [markers]);
  if (markers.length === 0) return null;
  return (
    <div className="mt-3 flex items-center gap-4 text-mono-sm text-muted-foreground">
      {(Object.keys(counts) as MarkerKind[]).map((kind) => (
        <span key={kind} className="flex items-center gap-1.5">
          <span
            className="inline-block h-3 w-px border-l border-dashed"
            style={{ borderColor: MARKER_STYLE[kind].stroke }}
          />
          {counts[kind]} {MARKER_STYLE[kind].label.toLowerCase()}{counts[kind]! > 1 ? "s" : ""}
        </span>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// X-axis helpers — produce evenly-spaced tick timestamps and a label
// formatter that switches between time-of-day and date depending on span.
// ---------------------------------------------------------------------------

function rangeMs(points: { t: number }[]): number {
  if (points.length < 2) return 0;
  return points[points.length - 1].t - points[0].t;
}

function buildTimeTicks(points: PodMetricPoint[]): number[] {
  if (points.length === 0) return [];
  const first = new Date(points[0].timestamp).getTime();
  const last = new Date(points[points.length - 1].timestamp).getTime();
  if (last <= first) return [first];
  // Aim for ~5 evenly-spaced ticks.
  const tickCount = 5;
  const ticks: number[] = [];
  for (let i = 0; i < tickCount; i++) {
    ticks.push(first + ((last - first) * i) / (tickCount - 1));
  }
  return ticks;
}

function formatTimeTick(spanMs: number): (value: number) => string {
  // < 36h → HH:MM. Longer ranges show a date (Mon 23) instead.
  const showDate = spanMs > 36 * 60 * 60 * 1000;
  return (value: number) => {
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return "";
    if (showDate) {
      return d.toLocaleDateString(undefined, { weekday: "short", day: "numeric" });
    }
    return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", hour12: false });
  };
}

function formatTooltipTimestamp(value: number): string {
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

// ---------------------------------------------------------------------------
// Value formatters
// ---------------------------------------------------------------------------

function formatCores(value: number): string {
  if (value < 0.01) return `${(value * 1000).toFixed(0)}m`;
  if (value < 1) return `${(value * 1000).toFixed(0)}m`;
  return value.toFixed(2);
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value.toFixed(0)} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(0)} KiB`;
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(0)} MiB`;
  return `${(value / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
}

function formatBytesPerSec(value: number): string {
  return `${formatBytes(value)}/s`;
}
