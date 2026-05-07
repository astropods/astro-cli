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
  const minAvg = Math.min(...active.map((p) => p.avgLatencyMs));
  const maxAvg = Math.max(...active.map((p) => p.avgLatencyMs));

  return { weightedAvg, peakP95, minAvg, maxAvg };
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
  const avg = stats?.weightedAvg ?? 0;
  const p95 = stats?.peakP95 ?? 0;
  const minAvg = stats?.minAvg ?? 0;
  const maxAvg = stats?.maxAvg ?? 0;

  return (
    <div className="flex h-full flex-col rounded-lg border border-border/60 bg-card dark:bg-surface p-5">
      {loading ? (
        <div className="flex min-h-[200px] flex-1 items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
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
              {formatLatency(avg)}
            </p>
          </div>

          {/* Divider */}
          <div className="my-4 border-t border-border/60" />

          {/* Secondary metrics */}
          <div className="grid grid-cols-2 gap-4">
            <div className="flex flex-col items-center gap-0.5">
              <p className="text-mono-sm text-muted-foreground">P95</p>
              <p className="font-mono text-lg font-medium text-foreground">
                {formatLatency(p95)}
              </p>
            </div>
            <div className="flex flex-col items-center gap-0.5">
              <p className="text-mono-sm text-muted-foreground">Range</p>
              <p className="font-mono text-lg font-medium text-foreground">
                {formatLatency(minAvg)}
                <span className="mx-1 text-muted-foreground">&ndash;</span>
                {formatLatency(maxAvg)}
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
