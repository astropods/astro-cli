import React from "react";
import { AreaChart, Area, ResponsiveContainer } from "recharts";
import { Card } from "@/components/ui/card";
import { cn } from "@/lib/utils";

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

export interface MetricCardProps {
  label: string;
  value: string;
  valueSuffix?: string;
  description?: React.ReactNode;
  trend?: number | null;
  higherIsBetter?: boolean;
  showTrend?: boolean;
  loading?: boolean;
  trendLoading?: boolean;
  sparkline?: number[];
  className?: string;
}

export function MetricCard({ label, value, valueSuffix, description, trend = null, higherIsBetter = true, showTrend = true, loading, trendLoading, sparkline, className }: MetricCardProps) {
  const chartData = sparkline?.map((v) => ({ v }));

  return (
    <Card className={cn("p-[12px_14px] dark:bg-surface", className)}>
      <span className={cn("block font-mono text-label uppercase tracking-[0.07em] text-faint-foreground", showTrend || sparkline || description ? "mb-2" : "mb-4")}>
        {label}
      </span>
      {loading ? (
        <SkeletonBar className="h-6 w-1/2" />
      ) : valueSuffix ? (
        <div className="flex items-baseline gap-1.5">
          <span className="font-sans text-heading-2 font-bold text-foreground">{value}</span>
          <span className="font-sans text-body-sm text-muted-foreground">{valueSuffix}</span>
        </div>
      ) : (
        <span className="block font-sans text-heading-2 font-bold text-foreground">{value}</span>
      )}
      {chartData && !loading && (
        <div className="mt-3 h-10">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
              <defs>
                <linearGradient id="sparkGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="var(--color-teal-600)" stopOpacity={0.15} />
                  <stop offset="100%" stopColor="var(--color-teal-600)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <Area type="monotone" dataKey="v" stroke="var(--color-teal-600)" strokeWidth={1.5} fill="url(#sparkGrad)" dot={false} isAnimationActive={true} animationDuration={1000} animationEasing="ease-out" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
      {description && !loading && !sparkline && (
        <p className="mt-3 text-body-sm text-muted-foreground">{description}</p>
      )}
      {showTrend && !sparkline && !description && (trendLoading ? (
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
