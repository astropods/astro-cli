import { useState, useMemo } from "react";
import { ChevronDown, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { CopyButton } from "@/components/ui/copy-button";
import { StatusBadge } from "@/components/StatusBadge";
import {
  MultiSelect,
  MultiSelectTrigger,
  MultiSelectValue,
  MultiSelectContent,
  MultiSelectAllItem,
  MultiSelectItem,
} from "@/components/ui/multi-select";
import type { TraceEntry } from "@/lib/api";
import {
  type TraceStatus,
  STATUS_CONFIG,
  STATUS_BADGE_COLOR,
  normalizeStatus,
  formatTimestamp,
  formatLatency,
  formatCost,
} from "./trace-utils";

const ALL_STATUSES: TraceStatus[] = ["success", "error", "timeout"];
const STATUS_OPTIONS = ALL_STATUSES.map((s) => ({
  value: s,
  label: STATUS_CONFIG[s].label,
}));
const DEFAULT_VISIBLE = 10;

// ---------------------------------------------------------------------------
// Table
// ---------------------------------------------------------------------------

export interface TracesTableProps {
  traces: TraceEntry[];
  loading?: boolean;
  selectedTraceId?: string | null;
  onSelectTrace?: (trace: TraceEntry) => void;
}

export function TracesTable({
  traces,
  loading,
  selectedTraceId,
  onSelectTrace,
}: TracesTableProps) {
  const [selectedStatuses, setSelectedStatuses] = useState<string[]>([]);
  const [expanded, setExpanded] = useState(false);

  const filtered = useMemo(
    () => selectedStatuses.length === 0
      ? traces
      : traces.filter((t) => selectedStatuses.includes(normalizeStatus(t.status))),
    [traces, selectedStatuses],
  );

  const visible = expanded ? filtered : filtered.slice(0, DEFAULT_VISIBLE);
  const hiddenCount = filtered.length - DEFAULT_VISIBLE;

  return (
    <div
      className="overflow-clip rounded-lg border border-border/60 bg-card dark:bg-surface"
    >
      {/* Filter bar */}
      <div className="flex items-center justify-between border-b border-border/60 px-4 py-3">
        <MultiSelect value={selectedStatuses} onValueChange={setSelectedStatuses}>
          <MultiSelectTrigger className="h-8 w-44 text-body-sm">
            <MultiSelectValue
              options={STATUS_OPTIONS}
              placeholder="All statuses"
            />
          </MultiSelectTrigger>
          <MultiSelectContent>
            <MultiSelectAllItem>All statuses</MultiSelectAllItem>
            {STATUS_OPTIONS.map((opt) => (
              <MultiSelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </MultiSelectItem>
            ))}
          </MultiSelectContent>
        </MultiSelect>
        <span className="text-mono-sm text-muted-foreground">
          {filtered.length} trace{filtered.length !== 1 ? "s" : ""}
        </span>
      </div>

      {loading ? (
        <div className="flex h-[200px] items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex h-[200px] items-center justify-center">
          <p className="text-body-sm text-muted-foreground">No traces found.</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[600px] border-collapse">
              <thead>
                <tr className="border-b border-border/60">
                  <th className="px-4 py-2 text-left text-mono-sm font-normal text-muted-foreground">Date</th>
                  <th className="px-4 py-2 text-left text-mono-sm font-normal text-muted-foreground">Status</th>
                  <th className="px-4 py-2 text-left text-mono-sm font-normal text-muted-foreground">Latency</th>
                  <th className="px-4 py-2 text-left text-mono-sm font-normal text-muted-foreground">Cost</th>
                  <th className="px-4 py-2 text-left text-mono-sm font-normal text-muted-foreground">Trace ID</th>
                </tr>
              </thead>
              <tbody>
                {visible.map((trace) => {
                  const status = normalizeStatus(trace.status);
                  const cfg = STATUS_CONFIG[status];
                  const isSelected = trace.trace_id === selectedTraceId;
                  return (
                    <tr
                      key={trace.trace_id}
                      onClick={() => onSelectTrace?.(trace)}
                      className={cn(
                        "group cursor-pointer border-b border-border/30 transition-colors",
                        isSelected
                          ? "bg-black/3 dark:bg-white/4"
                          : "hover:bg-black/2 dark:hover:bg-white/3",
                      )}
                    >
                      <td className="whitespace-nowrap px-4 py-2.5 text-body-sm text-foreground">
                        {formatTimestamp(trace.timestamp)}
                      </td>
                      <td className="px-4 py-2.5">
                        <StatusBadge color={STATUS_BADGE_COLOR[status]}>
                          {cfg.label}
                        </StatusBadge>
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5 font-mono text-body-sm text-foreground">
                        {formatLatency(trace.latency_ms)}
                      </td>
                      <td className="whitespace-nowrap px-4 py-2.5 font-mono text-body-sm text-muted-foreground">
                        {formatCost(trace.total_cost)}
                      </td>
                      <td className="px-4 py-2.5">
                        <span className="flex items-center gap-2">
                          <span className="font-mono text-body-sm text-muted-foreground">
                            {trace.trace_id}
                          </span>
                          <CopyButton
                            copyText={trace.trace_id}
                            title="Copy trace ID"
                            className="size-6 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
                            iconClassName="size-3"
                          />
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Expand / collapse */}
          {hiddenCount > 0 && (
            <button
              onClick={() => setExpanded((e) => !e)}
              className="flex w-full items-center justify-center gap-1.5 py-3 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <ChevronDown
                className={cn(
                  "size-3.5 transition-transform",
                  expanded && "rotate-180",
                )}
              />
              {expanded ? "Show less" : `Show ${hiddenCount} more`}
            </button>
          )}
        </>
      )}
    </div>
  );
}
