import { Loader2 } from "lucide-react";
import type { ChartColors, RequestVolumePoint } from "./chart-utils";
import { formatLatency as formatLatencyBase } from "../traces/trace-utils";

function formatLatency(ms: number) {
  return formatLatencyBase(ms, true);
}

function computeLatencyStats(points: RequestVolumePoint[]) {
  const active = points.filter((p) => p.requests > 0);
  if (active.length === 0) return null;

  const totalRequests = active.reduce((s, p) => s + p.requests, 0);
  const weightedAvg =
    active.reduce((s, p) => s + p.avgLatencyMs * p.requests, 0) / totalRequests;
  const peakP95 = Math.max(...active.map((p) => p.p95LatencyMs));
  const minLatency = Math.min(
    ...active.map((p) => p.minLatencyMs).filter((v) => v > 0),
  );
  const maxLatency = Math.max(...active.map((p) => p.maxLatencyMs));

  return {
    weightedAvg,
    peakP95,
    minLatency: Number.isFinite(minLatency) ? minLatency : 0,
    maxLatency,
  };
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export interface LatencyCardProps {
  points: RequestVolumePoint[];
  colors: ChartColors;
  loading?: boolean;
}

export function LatencyCard({
  points,
  colors,
  loading,
}: LatencyCardProps) {
  const stats = computeLatencyStats(points);

  return (
    <div className="flex h-full flex-col rounded-lg border border-border/60 bg-card p-5">
      {loading ? (
        <div className="flex min-h-[200px] flex-1 items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : stats === null ? (
        <div className="flex min-h-[200px] flex-1 flex-col items-center justify-center gap-1">
          <p className="text-mono-sm text-muted-foreground">Avg Latency</p>
          <p className="font-mono text-2xl text-muted-foreground">&mdash;</p>
          <p className="mt-1 text-body-sm text-muted-foreground">No requests in this range</p>
        </div>
      ) : (
        <div className="flex flex-1 flex-col">
          {/* Hero metric — avg latency */}
          <div className="flex flex-1 flex-col items-center justify-center">
            <p className="text-mono-sm text-muted-foreground">Avg Latency</p>
            <p
              className="mt-1 font-mono text-4xl font-semibold tracking-tight"
              style={{ color: colors.inputFill }}
            >
              {formatLatency(stats.weightedAvg)}
            </p>
          </div>

          {/* Divider */}
          <div className="my-4 border-t border-border/60" />

          {/* Secondary metrics */}
          <div className="grid grid-cols-3 gap-4">
            <div className="flex flex-col items-center gap-0.5">
              <p className="text-mono-sm text-muted-foreground">Min</p>
              <p className="font-mono text-lg font-medium text-foreground">
                {formatLatency(stats.minLatency)}
              </p>
            </div>
            <div className="flex flex-col items-center gap-0.5">
              <p className="text-mono-sm text-muted-foreground">P95</p>
              <p className="font-mono text-lg font-medium text-foreground">
                {formatLatency(stats.peakP95)}
              </p>
            </div>
            <div className="flex flex-col items-center gap-0.5">
              <p className="text-mono-sm text-muted-foreground">Max</p>
              <p className="font-mono text-lg font-medium text-foreground">
                {formatLatency(stats.maxLatency)}
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
