import { StatusBadge } from "@/components/StatusBadge";
import {
  STATUS_BADGE_COLOR,
  STATUS_CONFIG,
  formatCost,
  formatLatency,
  normalizeStatus,
} from "../trace-utils";

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
  const normalized = normalizeStatus(status ?? "success");
  const cfg = STATUS_CONFIG[normalized];

  return (
    <div className="border-b border-border px-4 py-3">
      <div className="grid grid-cols-4 gap-3">
        <MetaTile label="Status">
          <StatusBadge color={STATUS_BADGE_COLOR[normalized]}>
            {cfg.label}
          </StatusBadge>
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
