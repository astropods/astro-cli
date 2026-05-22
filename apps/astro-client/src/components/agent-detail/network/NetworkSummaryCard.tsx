import { Loader2 } from "lucide-react";
import type { NetworkDirectionSummary } from "@/lib/api";
import type { ChartColors } from "../charts/chart-utils";
import { formatCompactNumber } from "../charts/chart-utils";

function formatErrorRate(rate: number) {
  if (rate === 0) return "0%";
  if (rate < 0.001) return "<0.1%";
  return `${(rate * 100).toFixed(rate < 0.1 ? 2 : 1)}%`;
}

export interface NetworkSummaryCardProps {
  title: string;
  summary: NetworkDirectionSummary | undefined;
  colors: ChartColors;
  loading?: boolean;
  emptyMessage?: string;
}

export function NetworkSummaryCard({
  title,
  summary,
  colors,
  loading,
  emptyMessage = "No traffic in this range",
}: NetworkSummaryCardProps) {
  const hasTraffic = !!summary && summary.request_count > 0;

  return (
    <div className="flex h-full flex-col rounded-lg border border-border/60 bg-card dark:bg-surface p-5">
      <p className="text-mono-sm text-muted-foreground">{title}</p>

      {loading ? (
        <div className="flex min-h-[160px] flex-1 items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : !hasTraffic ? (
        <div className="flex min-h-[160px] flex-1 flex-col items-center justify-center gap-1">
          <p className="font-mono text-2xl text-muted-foreground">&mdash;</p>
          <p className="mt-1 text-body-sm text-muted-foreground">{emptyMessage}</p>
        </div>
      ) : (
        <div className="flex flex-1 flex-col">
          {/* Hero: total requests */}
          <div className="flex flex-1 flex-col items-center justify-center">
            <p
              className="mt-2 font-mono text-4xl font-semibold tracking-tight"
              style={{ color: colors.inputFill }}
            >
              {formatCompactNumber(summary.request_count)}
            </p>
            <p className="text-body-sm text-muted-foreground">requests</p>
          </div>

          <div className="my-4 border-t border-border/60" />

          <div className="grid grid-cols-2 gap-4">
            <div className="flex flex-col items-center gap-0.5">
              <p className="text-mono-sm text-muted-foreground">Errors</p>
              <p className="font-mono text-lg font-medium text-foreground">
                {formatErrorRate(summary.error_rate)}
              </p>
            </div>
            <div className="flex flex-col items-center gap-0.5">
              <p className="text-mono-sm text-muted-foreground">Peers</p>
              <p className="font-mono text-lg font-medium text-foreground">
                {summary.unique_peer_count}
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
