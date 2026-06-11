import { useEffect, useMemo, useState } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { CopyButton } from "@/components/ui/copy-button";
import { StatusBadge } from "@/components/StatusBadge";
import { TableShowMore } from "@/components/ui/table";
import {
  MultiSelect,
  MultiSelectTrigger,
  MultiSelectValue,
  MultiSelectContent,
  MultiSelectAllItem,
  MultiSelectItem,
} from "@/components/ui/multi-select";
import type { TraceEntry } from "@/lib/api";
import { TraceUserIdentity } from "./TraceUserIdentity";
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

const TRACE_ROW_CLASS = "group cursor-pointer border-b border-border/30 transition-colors";
const TRACE_ROW_SELECTED = "bg-black/3 dark:bg-white/4";
const TRACE_ROW_HOVER = "hover:bg-black/2 dark:hover:bg-white/3";

function shortTraceId(traceId: string) {
  if (traceId.length <= 24) return traceId;
  return `${traceId.slice(0, 12)}...${traceId.slice(-8)}`;
}

function TraceRowCells({ trace, account }: { trace: TraceEntry; account: string }) {
  const status = normalizeStatus(trace.status);
  const cfg = STATUS_CONFIG[status];
  const timestamp = formatTimestamp(trace.timestamp);
  return (
    <>
      <td className="truncate whitespace-nowrap px-3 py-2.5 text-body-sm text-foreground" title={timestamp}>
        {timestamp}
      </td>
      <td className="px-3 py-2.5">
        <StatusBadge color={STATUS_BADGE_COLOR[status]}>{cfg.label}</StatusBadge>
      </td>
      <td className="min-w-0 py-2.5 pl-3 pr-4">
        <div className="min-w-0 truncate">
          <TraceUserIdentity userId={trace.user_id} userDetails={trace.user_details} account={account} />
        </div>
      </td>
      <td className="truncate whitespace-nowrap py-2.5 pl-3 pr-2 font-mono text-body-sm text-foreground">
        {formatLatency(trace.latency_ms)}
      </td>
      <td className="truncate whitespace-nowrap px-2 py-2.5 font-mono text-body-sm text-muted-foreground">
        {formatCost(trace.total_cost)}
      </td>
      <td className="min-w-0 px-3 py-2.5" title={trace.trace_id}>
        <span className="flex min-w-0 max-w-full items-center gap-1.5">
          <span
            className="block min-w-0 truncate whitespace-nowrap font-mono text-mono-sm text-muted-foreground"
          >
            {shortTraceId(trace.trace_id)}
          </span>
          <CopyButton
            copyText={trace.trace_id}
            title="Copy trace ID"
            className="size-6 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
            iconClassName="size-3"
          />
        </span>
      </td>
    </>
  );
}

// ---------------------------------------------------------------------------
// Table
// ---------------------------------------------------------------------------

export interface TracesTableProps {
  traces: TraceEntry[];
  /** Account that owns the deployment — used to resolve trace user IDs to profiles. */
  account: string;
  loading?: boolean;
  selectedTraceId?: string | null;
  onSelectTrace?: (trace: TraceEntry) => void;
}

export function TracesTable({
  traces,
  account,
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

  const stableTraces = filtered.slice(0, DEFAULT_VISIBLE);
  const expandableTraces = filtered.slice(DEFAULT_VISIBLE);
  const hiddenCount = expandableTraces.length;
  // Filtering down to a list that fits in the visible window invalidates an
  // outstanding expanded state — reset so the next overflow starts collapsed.
  useEffect(() => {
    if (hiddenCount <= 0 && expanded) setExpanded(false);
  }, [hiddenCount, expanded]);

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
          <div className="overflow-hidden">
            <table className="w-full table-fixed border-collapse">
              <colgroup>
                <col className="w-[18%]" />
                <col className="w-[12%]" />
                <col className="w-[24%]" />
                <col className="w-[9%]" />
                <col className="w-[11%]" />
                <col className="w-[26%]" />
              </colgroup>
              <thead>
                <tr className="border-b border-border/60">
                  <th className="px-3 py-2 text-left text-mono-sm font-normal text-muted-foreground">Date</th>
                  <th className="px-3 py-2 text-left text-mono-sm font-normal text-muted-foreground">Status</th>
                  <th className="py-2 pl-3 pr-4 text-left text-mono-sm font-normal text-muted-foreground">User</th>
                  <th className="py-2 pl-3 pr-2 text-left text-mono-sm font-normal text-muted-foreground">Latency</th>
                  <th className="px-2 py-2 text-left text-mono-sm font-normal text-muted-foreground">Cost</th>
                  <th className="px-3 py-2 text-left text-mono-sm font-normal text-muted-foreground">Trace ID</th>
                </tr>
              </thead>
              <tbody>
                {stableTraces.map((trace) => {
                  const isSelected = trace.trace_id === selectedTraceId;
                  return (
                    <tr
                      key={trace.trace_id}
                      onClick={() => onSelectTrace?.(trace)}
                      className={cn(TRACE_ROW_CLASS, isSelected ? TRACE_ROW_SELECTED : TRACE_ROW_HOVER)}
                    >
                      <TraceRowCells trace={trace} account={account} />
                    </tr>
                  );
                })}
                {expanded &&
                  expandableTraces.map((trace) => {
                    const isSelected = trace.trace_id === selectedTraceId;
                    return (
                      <tr
                        key={trace.trace_id}
                        onClick={() => onSelectTrace?.(trace)}
                        className={cn(TRACE_ROW_CLASS, isSelected ? TRACE_ROW_SELECTED : TRACE_ROW_HOVER)}
                      >
                        <TraceRowCells trace={trace} account={account} />
                      </tr>
                    );
                  })}
              </tbody>
            </table>
          </div>

          <TableShowMore
            hiddenCount={hiddenCount}
            expanded={expanded}
            onToggle={() => setExpanded((e) => !e)}
          />

        </>
      )}
    </div>
  );
}
