import React, { useId } from "react";
import { AreaChart, Area, ResponsiveContainer, Tooltip } from "recharts";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { useResolvedTheme } from "@/lib/theme";
import { formatDateShort } from "@/lib/format-utils";

function SkeletonBar({ className }: { className?: string }) {
  return (
    <div className={cn("rounded animate-pulse bg-muted", className)} />
  );
}

function formatTrend(v: number | null): string {
  if (v === null || Number.isNaN(v) || !Number.isFinite(v)) return "—";
  return `${Math.round(Math.abs(v))}%`;
}

function TrendIndicator({ value, higherIsBetter }: { value: number | null; higherIsBetter: boolean }) {
  const dir = value === null ? "flat" : value > 0 ? "up" : value < 0 ? "down" : "flat";
  const arrow = dir === "up" ? "↑" : dir === "down" ? "↓" : "—";
  const isGood = dir === "flat" ? null : higherIsBetter ? dir === "up" : dir === "down";

  return (
    <div className={cn(
      "mt-2 flex items-center gap-1.5 font-sans text-body-sm font-bold",
      isGood === null ? "text-faint-foreground" : isGood ? "text-green-700 dark:text-green-400" : "text-coral-600 dark:text-coral-400",
    )}>
      <span>{formatTrend(value)}</span>
      <span>{arrow}</span>
    </div>
  );
}

// ChangeBadge renders inline next to the metric value, so toggling the date
// range only fades the badge contents in/out — the card's row height is
// pinned by the value's `text-heading-2`, never by the badge's presence.
// Zero change (value === 0) is treated as no change: no badge shown.
function ChangeBadge({ value, label, visible }: { value: number | null | undefined; label?: string; visible: boolean }) {
  const hasChange = value !== null && value !== undefined && value !== 0;
  const show = visible && hasChange;
  const up = hasChange && value! > 0;
  return (
    <span
      className={cn(
        "inline-flex items-baseline gap-1 font-mono text-label uppercase tracking-[0.07em] transition-opacity duration-200",
        show ? "opacity-100" : "opacity-0",
      )}
      aria-hidden={!show}
    >
      {hasChange ? (
        <>
          <span className={up ? "text-success" : "text-destructive"}>
            {up ? "↑" : "↓"} {Math.abs(Math.round(value!))}%
          </span>
          {label && <span className="normal-case tracking-normal text-faint-foreground">{label}</span>}
        </>
      ) : null}
    </span>
  );
}

interface SparkEntry { v: number; date?: string }

function SparkTooltip({
  active,
  payload,
  formatValue,
}: {
  active?: boolean;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload?: ReadonlyArray<any>;
  formatValue?: (v: number) => string;
}) {
  if (!active || !payload?.length) return null;
  const entry = payload[0]?.payload as SparkEntry;
  const formatted = formatValue ? formatValue(entry.v) : String(Math.round(entry.v));
  return (
    <div className="rounded-md border border-border bg-popover px-2 py-1.5 shadow-md">
      {entry.date && (
        <p className="font-mono text-mono-sm text-muted-foreground">{formatDateShort(entry.date)}</p>
      )}
      <p className="font-mono text-body-sm font-medium text-foreground">{formatted}</p>
    </div>
  );
}

export interface MetricCardProps {
  label: string;
  value: string;
  valueSuffix?: string;
  description?: React.ReactNode;

  // Bold-style trend (used by DashboardStats)
  trend?: number | null;
  higherIsBetter?: boolean;
  showTrend?: boolean;
  trendLoading?: boolean;

  // Label-style change (used by activity StatCards). Renders instead of
  // TrendIndicator when `changePct` is provided.
  changePct?: number | null;
  changeLabel?: string;
  showChange?: boolean;

  sparkline?: number[];
  // When provided, the sparkline gets an interactive tooltip showing the
  // date at the cursor position alongside the formatted value.
  sparklineDates?: string[];
  formatSparkValue?: (v: number) => string;

  subValues?: { label: string; value: string }[];

  loading?: boolean;
  className?: string;
}

export function MetricCard({
  label,
  value,
  valueSuffix,
  description,
  trend = null,
  higherIsBetter = true,
  showTrend = true,
  trendLoading,
  changePct,
  changeLabel,
  showChange,
  sparkline,
  sparklineDates,
  formatSparkValue,
  subValues,
  loading,
  className,
}: MetricCardProps) {
  const uid = useId();
  const gradId = `spark-grad-${uid}`;
  const resolvedTheme = useResolvedTheme();
  const isDark = resolvedTheme === "dark";
  const sparkColor = isDark ? "var(--color-teal-500)" : "var(--color-indigo-600)";

  const chartData = sparkline?.map((v, i) => ({ v, date: sparklineDates?.[i] }));
  const interactiveSpark = sparklineDates !== undefined;
  const hasChangeApi = changePct !== undefined;
  const hasSubValues = subValues && subValues.length > 0;
  const renderTrend = showTrend && !sparkline && !description && !hasChangeApi && !hasSubValues;

  return (
    <Card className={cn("p-[12px_14px] dark:bg-surface", className)}>
      <span className={cn(
        "block font-mono text-label uppercase tracking-[0.07em] text-faint-foreground",
        renderTrend || sparkline || description || hasChangeApi || hasSubValues ? "mb-2" : "mb-4",
      )}>
        {label}
      </span>
      {loading ? (
        <SkeletonBar className="h-6 w-1/2" />
      ) : (
        // Value + (optional suffix) + (optional change badge) on a single
        // baseline-aligned row. Putting the change badge inline pins the
        // card row height to the heading-2 line so toggling between ranges
        // never grows or shrinks the card.
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
          <span className="font-sans text-heading-2 font-bold text-foreground">{value}</span>
          {valueSuffix && (
            <span className="font-sans text-body-sm text-muted-foreground">{valueSuffix}</span>
          )}
          {hasChangeApi && (
            <ChangeBadge value={changePct} label={changeLabel} visible={showChange !== false} />
          )}
        </div>
      )}
      {!loading && hasSubValues && (
        <div className="mt-1 flex gap-3">
          {subValues!.map((sv) => (
            <span key={sv.label} className="font-mono text-mono-sm text-faint-foreground">
              <span className="text-muted-foreground">{sv.value}</span> {sv.label}
            </span>
          ))}
        </div>
      )}
      {chartData && chartData.length > 1 && !loading && (
        <div className={cn("mt-3 h-10", interactiveSpark && "[&_.recharts-surface]:overflow-visible")}>
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart
              data={chartData}
              margin={interactiveSpark ? { top: 6, right: 2, bottom: 0, left: 2 } : { top: 0, right: 0, bottom: 0, left: 0 }}
            >
              <defs>
                <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={sparkColor} stopOpacity={isDark ? 0.35 : 0.25} />
                  <stop offset="100%" stopColor={sparkColor} stopOpacity={0} />
                </linearGradient>
              </defs>
              {interactiveSpark && (
                <Tooltip
                  content={(props) => <SparkTooltip {...props} formatValue={formatSparkValue} />}
                  cursor={{ stroke: sparkColor, strokeWidth: 1, strokeDasharray: "3 3" }}
                />
              )}
              <Area
                type="monotone"
                dataKey="v"
                stroke={sparkColor}
                strokeWidth={1.5}
                fill={`url(#${gradId})`}
                dot={false}
                isAnimationActive
                animationDuration={interactiveSpark ? 800 : 1000}
                animationEasing="ease-out"
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
      {description && !loading && !sparkline && (
        <p className="mt-3 text-body-sm text-muted-foreground">{description}</p>
      )}
      {renderTrend && (trendLoading ? (
        <div className="mt-2 flex gap-1.5 items-center">
          <SkeletonBar className="h-3.5 w-1/4" />
          <SkeletonBar className="h-3.5 w-8" />
          <SkeletonBar className="h-3.5 w-1/3" />
        </div>
      ) : (
        <TrendIndicator value={trend} higherIsBetter={higherIsBetter} />
      ))}
    </Card>
  );
}
