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
  trend?: number | null;
  higherIsBetter?: boolean;
  showTrend?: boolean;
  loading?: boolean;
  trendLoading?: boolean;
  className?: string;
}

export function MetricCard({ label, value, trend = null, higherIsBetter = true, showTrend = true, loading, trendLoading, className }: MetricCardProps) {
  return (
    <div className={cn("rounded-[10px] border border-border bg-surface p-[12px_14px]", className)}>
      <span className="mb-2 block font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">
        {label}
      </span>
      {loading ? (
        <SkeletonBar className="h-6 w-1/2" />
      ) : (
        <span className="block font-sans text-heading-2 font-bold text-foreground">{value}</span>
      )}
      {showTrend && (trendLoading ? (
        <div className="mt-2 flex gap-1.5 items-center">
          <SkeletonBar className="h-3.5 w-1/4" />
          <SkeletonBar className="h-3.5 w-8" />
          <SkeletonBar className="h-3.5 w-1/3" />
        </div>
      ) : (
        <TrendIndicator value={trend} higherIsBetter={higherIsBetter} />
      ))}
    </div>
  );
}
