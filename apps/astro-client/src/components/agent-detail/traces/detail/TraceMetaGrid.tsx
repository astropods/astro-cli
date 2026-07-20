import { formatCost, formatLatency } from "../trace-utils";

export interface TraceMetaGridProps {
  latencyMs: number;
  totalCost?: number;
  totalTokens?: number;
}

export function TraceMetaGrid({ latencyMs, totalCost, totalTokens }: TraceMetaGridProps) {
  return (
    <div className="border-b border-border px-4 py-3">
      <div className="grid grid-cols-3 gap-3">
        <MetaTile label="Latency">
          <span className="font-mono text-body-sm text-foreground">
            {formatLatency(latencyMs, true)}
          </span>
        </MetaTile>
        <MetaTile label="Cost">
          <span className="font-mono text-body-sm text-foreground">
            {formatCost(totalCost)}
          </span>
        </MetaTile>
        <MetaTile label="Tokens">
          <span className="font-mono text-body-sm text-foreground">
            {totalTokens != null && totalTokens > 0
              ? totalTokens.toLocaleString()
              : "—"}
          </span>
        </MetaTile>
      </div>
    </div>
  );
}

function MetaTile({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col items-start gap-1">
      <span className="text-mono-sm text-muted-foreground/60">{label}</span>
      {children}
    </div>
  );
}
