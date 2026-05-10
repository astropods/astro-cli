import { STATUS_CONFIG, formatCost, formatLatency, normalizeStatus } from "../trace-utils";

export interface TraceMetaGridProps {
  status?: string;
  latencyMs: number;
  totalCost?: number;
  totalTokens?: number;
}

export function TraceMetaGrid({
  status,
  latencyMs,
  totalCost,
  totalTokens,
}: TraceMetaGridProps) {
  const cfg = STATUS_CONFIG[normalizeStatus(status ?? "success")];

  return (
    <div className="border-b border-border px-4 py-3">
      <div className="grid grid-cols-4 gap-3">
        <MetaTile label="Status">
          <span
            className="inline-flex items-center gap-[5px] rounded border pl-[6px] pr-[10px] py-1 font-mono text-label font-normal tracking-[0.06em]"
            style={{ background: cfg.bg, borderColor: cfg.bdr, color: cfg.fg }}
          >
            {cfg.label}
          </span>
        </MetaTile>
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
