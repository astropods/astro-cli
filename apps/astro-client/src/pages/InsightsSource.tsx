import { useCallback, useEffect, useMemo, useState, type MouseEvent as ReactMouseEvent } from "react";
import { Link, useParams } from "react-router";
import {
  ResponsiveContainer,
  BarChart,
  Bar,
  Cell,
  Rectangle,
  Pie,
  PieChart,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";
import { ArrowLeft, Bot, Info } from "lucide-react";
import { usePersistentSearchParams } from "@/hooks/use-persistent-search-params";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { useAccountInsightsSource } from "@/api/queries/observability";
import { observabilityKeys } from "@/api/queries/keys";
import { getPageAccount } from "@/lib/api.server";
import { getIntegrationIconUrl } from "@/lib/assets";
import { useResolvedTheme } from "@/lib/theme";
import { formatCost, formatCompact, formatDateShort } from "@/lib/format-utils";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/card";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { AccountScopeFilter } from "@/components/AccountScopeFilter";
import { TimeRangeSelector } from "@/components/activity/TimeRangeSelector";
import { PillToggle } from "@/components/activity/PillToggle";
import { SettledContentReveal } from "@/components/ui/content-reveal";
import { categoryColor } from "@/components/activity/model-colors";
import type { InsightsSourceAxis, InsightsSourceLabel, InsightsSourceResponse } from "@/lib/api";
import { useInsightsScopeAccount } from "./insights-account-param";
import { PeopleTable, type SourceFilter } from "./InsightsSourcePeople";
import { topSegmentKey } from "@/components/activity/stacked-bars";
import {
  GRID_PROPS,
  SeriesLegend,
  SeriesTooltip,
  X_AXIS_BASE,
  useHiddenSeries,
  yAxisProps,
} from "@/components/activity/chart-chrome";
import type { Route } from "./+types/InsightsSource";

const SOURCE_FILTER_PARAMS = ["range", "metric"] as const;
const DEFAULT_RANGE = "30d";

// Prompt share is exact; per-prompt cost is a developer's day smeared evenly.
type SourceMetric = "traces" | "cost";
const METRIC_OPTIONS: { key: SourceMetric; label: string }[] = [
  { key: "traces", label: "Prompts" },
  { key: "cost", label: "Spend" },
];

// Keyed off colour_index, not list position, which the fold reorders per range.
function buildLabelColors(
  axisKey: string,
  axis: InsightsSourceAxis | undefined,
): Record<string, string> {
  const map: Record<string, string> = {};
  axis?.labels.forEach((l) => { map[l.key] = categoryColor(axisKey, l.color_index); });
  return map;
}

// getPageAccount resolves ?account= against the caller's memberships.
export async function loader({ request, params }: Route.LoaderArgs) {
  const ctx = await getPageAccount(request);
  const source = params.source ?? "";
  if (!ctx || !source) return { account: null, source, insights: null };
  const insights = await ctx.api
    .getAccountInsightsSource(ctx.accountName, source)
    .catch(() => null);
  return { account: ctx.accountName, source, insights };
}

// Only an account change is different data; the rest slice a loaded payload.
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

export default function InsightsSource({ loaderData }: Route.ComponentProps) {
  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (!ld?.account || !ld.insights) return;
    qc.setQueryData(observabilityKeys.insightsSource(ld.account, ld.source), ld.insights);
  });

  const { source = "" } = useParams();
  const [searchParams, setSearchParams] =
    usePersistentSearchParams("insights-source", SOURCE_FILTER_PARAMS);
  const resolvedTheme = useResolvedTheme();

  const { account, paramAccount, setScopeAccount } = useInsightsScopeAccount(searchParams, setSearchParams);

  // Separate key: the day narrows only the table, and drilling back out is cached.
  const [day, setDay] = useState<string | null>(null);
  const { data: payload, isError } = useAccountInsightsSource(account, source);
  const dayQuery = useAccountInsightsSource(account, source, { day: day ?? undefined, enabled: !!day });

  const range = searchParams.get("range") ?? DEFAULT_RANGE;
  const costUnavailable = payload?.coverage.cost_unavailable ?? false;
  // Every axis with data, in declared order; all render together.
  const shownAxes = useMemo(() => {
    const byKey = payload?.ranges[range]?.axes ?? {};
    return (payload?.axes ?? [])
      .map((ref) => ({ ref, axis: byKey[ref.key] }))
      .filter((entry): entry is { ref: { key: string; label: string }; axis: InsightsSourceAxis } =>
        Boolean(entry.axis));
  }, [payload, range]);
  const metric: SourceMetric =
    costUnavailable ? "traces" : (searchParams.get("metric") === "cost" ? "cost" : "traces");

  const setParam = (key: string, value: string) =>
    setSearchParams((prev) => { prev.set(key, value); return prev; }, { replace: true });

  // Clears on range change, where the clicked segment may not survive the fold.
  const [filter, setFilter] = useState<SourceFilter | null>(null);
  useEffect(() => { setFilter(null); setDay(null); }, [range, account, source]);
  const selectCategory = useCallback(
    (ref: { key: string; label: string }, label: { key: string; label: string; color_index: number }) =>
      setFilter((current) =>
        current && current.axisKey === ref.key && current.labelKey === label.key
          ? null // clicking the selected segment again clears it
          : {
            axisKey: ref.key,
            axisLabel: ref.label,
            labelKey: label.key,
            labelName: label.label,
            colorIndex: label.color_index,
          }),
    [],
  );

  // Keyed on the fetch, so slicing the loaded payload does not re-trigger it.
  const revealKey = `${account}:${source}`;
  const icon = payload?.source.icon;
  const label = payload?.source.label ?? source;

  // Same surface as Insights, which links here.
  return (
    <PageContainer outerClassName="bg-background">
      <Link
        to={paramAccount ? `/insights?account=${encodeURIComponent(paramAccount)}` : "/insights"}
        className="mb-4 inline-flex items-center gap-1.5 text-body-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="size-3.5" aria-hidden />
        Insights
      </Link>

      <PageHeader
        title={
          <span className="inline-flex items-center gap-2">
            {icon
              ? <img src={getIntegrationIconUrl(icon, resolvedTheme)} alt="" className="size-7 shrink-0 object-contain" />
              : <Bot className="size-7 shrink-0 text-muted-foreground" aria-hidden />}
            {label}
          </span>
        }
        adornment={<AccountScopeFilter value={account} onChange={setScopeAccount} className="-ml-1" />}
        action={
          <div className="flex flex-wrap items-center gap-2">
            {!costUnavailable && (
              <PillToggle
                value={metric}
                options={METRIC_OPTIONS}
                onChange={(next) => setParam("metric", next)}
                layoutId="source-metric-pill"
                className="w-fit"
              />
            )}
            <TimeRangeSelector value={range} onChange={(r) => setParam("range", r)} />
          </div>
        }
      />

      {isError ? (
        <EmptyCard title="Couldn't load this source" body="Try again in a moment." />
      ) : (
        <SettledContentReveal transitionKey={revealKey} settled={!!payload}>
          <div className="flex flex-col gap-6">
            {payload && <CoverageNotice payload={payload} />}

            {shownAxes.length > 0 ? (
              shownAxes.map(({ ref, axis }) => (
                <section key={ref.key} className="flex flex-col gap-3">
                  <h2 className="text-heading-3 text-foreground">{ref.label}</h2>
                  <div className="grid gap-4 @4xl:grid-cols-[1fr_360px]">
                    <LabelChart
                      axisKey={ref.key}
                      axis={axis}
                      metric={metric}
                      filter={filter}
                      selectedDay={day}
                      onSelectDay={(next) => setDay((current) => (current === next ? null : next))}
                      onSelect={(label) => selectCategory(ref, label)}
                    />
                    <LabelTable
                      axisKey={ref.key}
                      axis={axis}
                      metric={metric}
                      filter={filter}
                      onSelect={(label) => selectCategory(ref, label)}
                    />
                  </div>
                </section>
              ))
            ) : (
              <EmptyCard
                title="Nothing classified yet"
                body={
                  payload?.coverage.content_available === false
                    ? "Prompt collection is turned off for this tool, so there are no prompts to categorise. It's enabled in your Anthropic admin console, not in Astro AI."
                    : "No prompts in this range have been categorised yet."
                }
              />
            )}

            {shownAxes.length > 0 && payload?.ranges[range] && (
              <PeopleTable
                people={(day ? dayQuery.data?.ranges[range]?.people : payload.ranges[range].people)
                  ?? payload.ranges[range].people}
                filter={filter}
                onClearFilter={() => setFilter(null)}
                day={day}
                onClearDay={() => setDay(null)}
                loading={!!day && dayQuery.isFetching && !dayQuery.data}
                axes={payload.axes}
              />
            )}
          </div>
        </SettledContentReveal>
      )}
    </PageContainer>
  );
}

// A partial window has to be stated, or it reads as "nobody used this".
function CoverageNotice({ payload }: { payload: InsightsSourceResponse }) {
  const { coverage } = payload;
  const notes: string[] = [];
  if (!coverage.backfill_complete && coverage.classified_from) {
    notes.push(`Still categorising older history — complete from ${coverage.classified_from}.`);
  }
  if (coverage.cost_unavailable) {
    notes.push("Spend isn't available for this tool's model yet, so categories are shown by prompt count.");
  }
  if (notes.length === 0) return null;

  return (
    <div className="flex items-start gap-2 rounded-md border border-border bg-surface px-3 py-2">
      <Info className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" aria-hidden />
      <div className="flex flex-col gap-0.5">
        {notes.map((n) => (
          <p key={n} className="text-body-sm text-muted-foreground">{n}</p>
        ))}
      </div>
    </div>
  );
}

function LabelChart({
  axisKey,
  axis,
  metric,
  filter,
  selectedDay,
  onSelectDay,
  onSelect,
}: {
  axisKey: string;
  axis: InsightsSourceAxis;
  metric: SourceMetric;
  filter: SourceFilter | null;
  selectedDay: string | null;
  onSelectDay: (day: string) => void;
  onSelect: (label: InsightsSourceLabel) => void;
}) {
  // Dim, not hide: removing the others would rescale the axis mid-read.
  const dimmed = (key: string) =>
    filter !== null && !(filter.axisKey === axisKey && filter.labelKey === key);
  const colors = useMemo(() => buildLabelColors(axisKey, axis), [axisKey, axis]);
  const keys = useMemo(() => axis.labels.map((l) => l.key), [axis.labels]);
  const names = useMemo(
    () => Object.fromEntries(axis.labels.map((l) => [l.key, l.label])),
    [axis.labels],
  );

  const { hidden, visible, toggle } = useHiddenSeries(keys);

  // Zeros included, so a segment absent for a day keeps its slot in the stack.
  const rows = useMemo(
    () => axis.series.map((point) => {
      // Axis label plus the ISO day a click reports.
      const row: Record<string, string | number> = {
        date: formatDateShort(point.date),
        iso: point.date,
      };
      const values = metric === "cost" ? point.cost_usd : point.traces;
      for (const key of visible) row[key] = values[key] ?? 0;
      row.cap = topSegmentKey(row, visible);
      return row;
    }),
    [axis.series, visible, metric],
  );
  const formatValue = metric === "cost" ? formatCost : formatCompact;

  // Recharts retains rendered rectangles to animate between them, so the stack
  // composition goes in the key to force a remount when the rounded cap moves.
  const stackSignature = visible.join("|");

  return (
    <Card className="flex h-[340px] flex-col p-5">
      <h3 className="mb-4 shrink-0 text-heading-4 text-foreground">
        {metric === "cost" ? "Spend by category" : "Prompts by category"}
      </h3>
      <div className="min-h-0 flex-1 [&_*]:outline-none">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart
            data={rows}
            margin={{ top: 16, right: 52, bottom: 4, left: 0 }}
            barCategoryGap="20%"
            maxBarSize={56}
            onClick={(state) => {
              const index = (state as { activeTooltipIndex?: number } | undefined)?.activeTooltipIndex;
              const iso = index == null ? undefined : rows[index]?.iso;
              if (typeof iso === "string") onSelectDay(iso);
            }}
            className="cursor-pointer"
          >
            <CartesianGrid {...GRID_PROPS} />
            <XAxis {...X_AXIS_BASE} />
            <YAxis {...yAxisProps(formatValue)} />
            <Tooltip
              content={<SeriesTooltip colors={colors} names={names} format={formatValue} />}
              cursor={{ fill: "var(--color-border)", fillOpacity: 0.3 }}
            />
            {visible.map((key) => (
              <Bar
                key={`${key}:${stackSignature}`}
                dataKey={key}
                stackId="value"
                fill={colors[key]}
                fillOpacity={dimmed(key) ? 0.18 : 0.85}
                shape={(props: BarShapeProps) => (
                  <Rectangle
                    {...props}
                    // Unselected days recede so the chosen column reads as the table's.
                    fillOpacity={
                      selectedDay && props.payload?.iso !== selectedDay
                        ? 0.18
                        : dimmed(key) ? 0.18 : 0.85
                    }
                    radius={props.payload?.cap === key ? [3, 3, 0, 0] : 0}
                  />
                )}
                isAnimationActive
                animationDuration={500}
                animationEasing="ease-out"
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>
      <SeriesLegend
        keys={keys}
        colors={colors}
        names={names}
        hidden={hidden}
        onToggle={toggle}
        onSelect={(key) => onSelect(labelBy(axis, key))}
      />
    </Card>
  );
}

function labelBy(axis: InsightsSourceAxis, key: string): InsightsSourceLabel {
  return axis.labels.find((l) => l.key === key) ?? axis.labels[0];
}

// Recharts types the shape callback loosely; this is the slice we read.
type BarShapeProps = { payload?: { iso?: string; cap?: string } };


function LabelTable({
  axisKey,
  axis,
  metric,
  filter,
  onSelect,
}: {
  axisKey: string;
  axis: InsightsSourceAxis;
  metric: SourceMetric;
  filter: SourceFilter | null;
  onSelect: (label: InsightsSourceLabel) => void;
}) {
  const dimmed = (key: string) =>
    filter !== null && !(filter.axisKey === axisKey && filter.labelKey === key);
  const colors = useMemo(() => buildLabelColors(axisKey, axis), [axisKey, axis]);
  // Dropped from the ring only — a zero-spend category still belongs in the legend.
  const slices = useMemo(
    () => axis.labels
      .map((l) => ({ ...l, value: metric === "cost" ? l.cost_usd : l.traces }))
      .filter((d) => d.value > 0),
    [axis.labels, metric],
  );
  const total = metric === "cost" ? axis.totals.cost_usd : axis.totals.traces;
  const formatValue = metric === "cost" ? formatCost : formatCompact;

  // A pie has no cartesian cursor, so Recharts eases between sector midpoints;
  // driving position from the pointer makes the tooltip track it instead.
  const [cursor, setCursor] = useState<{ x: number; y: number } | null>(null);
  const trackCursor = useCallback((e: ReactMouseEvent<HTMLDivElement>) => {
    const box = e.currentTarget.getBoundingClientRect();
    setCursor({ x: e.clientX - box.left + 12, y: e.clientY - box.top + 12 });
  }, []);

  return (
    <Card className="flex flex-col p-5">
      <h3 className="mb-2 shrink-0 text-heading-4 text-foreground">Breakdown</h3>

      <div
        className="relative h-[180px] shrink-0 [&_*]:outline-none"
        onMouseMove={trackCursor}
        onMouseLeave={() => setCursor(null)}
      >
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={slices}
              dataKey="value"
              nameKey="label"
              innerRadius={DONUT_INNER}
              outerRadius={DONUT_OUTER}
              paddingAngle={2}
              stroke="none"
              isAnimationActive
              animationDuration={500}
              animationEasing="ease-out"
            >
              {slices.map((d) => (
                <Cell
                  key={d.key}
                  fill={colors[d.key]}
                  fillOpacity={dimmed(d.key) ? 0.18 : 0.9}
                  className="cursor-pointer"
                  onClick={() => onSelect(d)}
                />
              ))}
            </Pie>
            <Tooltip
              content={<DonutTooltip format={formatValue} total={total} colors={colors} />}
              cursor={false}
              isAnimationActive={false}
              position={cursor ?? undefined}
              allowEscapeViewBox={{ x: true, y: true }}
              wrapperStyle={{ pointerEvents: "none", zIndex: 10 }}
            />
          </PieChart>
        </ResponsiveContainer>
        <DonutCenter value={formatValue(total)} caption={metric === "cost" ? "spend" : "prompts"} />
      </div>

      <ul className="mt-3 flex flex-col gap-1.5">
        {axis.labels.map((l) => {
          const pct = metric === "cost" ? l.cost_pct : l.traces_pct;
          const value = metric === "cost" ? formatCost(l.cost_usd) : formatCompact(l.traces);
          const isDim = dimmed(l.key);
          return (
            <li key={l.key}>
              <button
                type="button"
                onClick={() => onSelect(l)}
                className={cn(
                  "flex w-full items-center gap-2 rounded px-1 py-0.5 text-left transition-colors",
                  "hover:bg-muted focus-visible:bg-muted focus-visible:outline-none",
                  isDim && "opacity-45",
                )}
              >
              <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: colors[l.key] }} />
              <span className="truncate text-body-sm text-foreground">{l.label}</span>
              {l.aggregated && (
                <span className="shrink-0 text-mono-sm text-faint-foreground">rolled up</span>
              )}
              <span className="ml-auto shrink-0 font-mono text-mono-sm text-muted-foreground">{value}</span>
              <span className="w-9 shrink-0 text-right font-mono text-mono-sm text-faint-foreground">
                {pct.toFixed(0)}%
              </span>
              </button>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

// Fixed so the hole is a known size to lay the centre against.
const DONUT_INNER = 56;
const DONUT_OUTER = 82;

// HTML overlay, not a Recharts <Label>: in Recharts 3 a Pie label with custom
// content is not reliably handed the polar viewBox and lays out at the origin.
// Width is estimated from character count — the face is monospace.
const MONO_ADVANCE = 0.62;
// Largest square that fits inside the hole, less a little breathing room.
const CENTRE_BOX = DONUT_INNER * Math.SQRT2 * 0.9;

// Not aria-hidden: the total appears nowhere else on the tile.
function DonutCenter({ value, caption }: { value: string; caption: string }) {
  const size = Math.max(11, Math.min(22, CENTRE_BOX / Math.max(value.length, 1) / MONO_ADVANCE));
  return (
    <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-0.5">
      <span
        className="max-w-full truncate font-mono font-medium text-foreground tabular-nums"
        style={{ fontSize: size, maxWidth: CENTRE_BOX }}
      >
        {value}
      </span>
      <span
        className="max-w-full truncate font-mono text-mono-sm text-muted-foreground"
        style={{ maxWidth: CENTRE_BOX }}
      >
        {caption}
      </span>
    </div>
  );
}

function DonutTooltip({
  active,
  payload,
  format,
  total,
  colors,
}: {
  active?: boolean;
  payload?: { name?: string; value?: number; payload?: { key: string } }[];
  format: (v: number) => string;
  total: number;
  colors: Record<string, string>;
}) {
  const slice = payload?.[0];
  if (!active || !slice || slice.value == null) return null;
  const pct = total > 0 ? (slice.value / total) * 100 : 0;
  return (
    <div className="rounded-md border border-border bg-surface/95 px-3 py-2 shadow-lg backdrop-blur">
      <div className="flex items-center gap-2">
        <span
          className="size-2 shrink-0 rounded-full"
          style={{ backgroundColor: colors[slice.payload?.key ?? ""] }}
        />
        <span className="font-mono text-body-sm text-muted-foreground">{slice.name}</span>
        <span className="ml-auto font-mono text-body-sm font-medium text-foreground">
          {format(slice.value)}
        </span>
      </div>
      <p className="mt-0.5 pl-4 font-mono text-mono-sm text-faint-foreground">
        {pct.toFixed(0)}% of total
      </p>
    </div>
  );
}

function EmptyCard({ title, body }: { title: string; body: string }) {
  return (
    <Card className={cn("flex flex-col items-center justify-center gap-1 p-10 text-center")}>
      <p className="text-body text-foreground">{title}</p>
      <p className="max-w-md text-body-sm text-muted-foreground">{body}</p>
    </Card>
  );
}
