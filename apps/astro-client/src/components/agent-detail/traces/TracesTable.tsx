import { useEffect, useMemo, useState } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { CopyButton } from "@/components/ui/copy-button";
import { FilterInput } from "@/components/FilterInput";
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
import { traceRowAnchorId } from "@/lib/routes";
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

const TRACE_ROW_CLASS = "group cursor-pointer border-b border-border/30 transition-colors";
const TRACE_ROW_SELECTED = "bg-black/3 dark:bg-white/4";
const TRACE_ROW_HOVER = "hover:bg-black/2 dark:hover:bg-white/3";

function shortTraceId(traceId: string) {
  if (traceId.length <= 16) return traceId;
  return `...${traceId.slice(-8)}`;
}

// Case-insensitive match across the fields a user is most likely to search by.
// The name is included even though it isn't a visible column, since users often
// know a trace by its span name.
function traceMatchesSearch(trace: TraceEntry, query: string): boolean {
  const haystack = [
    trace.name,
    trace.trace_id,
    trace.user_id,
    // The User column shows display_name/username, not the raw id, so search
    // must include them or a search by the visible name returns nothing.
    trace.user_details?.display_name,
    trace.user_details?.username,
    STATUS_CONFIG[normalizeStatus(trace.status)]?.label,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
  return haystack.includes(query);
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
      <td className="min-w-0 px-3 py-2.5 text-right" title={trace.trace_id}>
        <span className="flex min-w-0 max-w-full items-center justify-end gap-2">
          <span
            className="block min-w-0 overflow-hidden text-clip whitespace-nowrap text-right font-mono text-mono-sm text-muted-foreground"
          >
            {shortTraceId(trace.trace_id)}
          </span>
          <CopyButton
            copyText={trace.trace_id}
            title="Copy trace ID"
            className="pointer-events-none size-6 shrink-0 rounded-sm opacity-0 transition-opacity group-hover:pointer-events-auto group-hover:opacity-100 focus-visible:pointer-events-auto focus-visible:opacity-100"
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
  /** Another page of traces exists on the server past the loaded window. */
  hasMore?: boolean;
  /** Fetch the next server page; appended to the loaded window. */
  onLoadMore?: () => void;
  /** The next page is in flight. */
  loadingMore?: boolean;
}

export function TracesTable({
  traces,
  account,
  loading,
  selectedTraceId,
  onSelectTrace,
  hasMore,
  onLoadMore,
  loadingMore,
}: TracesTableProps) {
  const [selectedStatuses, setSelectedStatuses] = useState<string[]>([]);
  const [search, setSearch] = useState("");

  const query = search.trim().toLowerCase();
  const filtered = useMemo(
    () =>
      traces.filter((t) => {
        if (
          selectedStatuses.length > 0 &&
          !selectedStatuses.includes(normalizeStatus(t.status))
        ) {
          return false;
        }
        if (query && !traceMatchesSearch(t, query)) return false;
        return true;
      }),
    [traces, selectedStatuses, query],
  );

  const selectedTraceIndex = selectedTraceId
    ? filtered.findIndex((trace) => trace.trace_id === selectedTraceId)
    : -1;
  useEffect(() => {
    if (!selectedTraceId || selectedTraceIndex < 0) return;

    const frame = window.requestAnimationFrame(() => {
      const row = document.getElementById(traceRowAnchorId(selectedTraceId));
      if (!row) return;
      // Only recenter when the row isn't already fully visible. A plain
      // click-to-select on an in-view row shouldn't jump the viewport; this
      // keeps the recenter behavior for the deep-link case where the row
      // lands off-screen.
      const rect = row.getBoundingClientRect();
      const fullyVisible = rect.top >= 0 && rect.bottom <= window.innerHeight;
      if (fullyVisible) return;
      row.scrollIntoView({ block: "center", inline: "nearest" });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [selectedTraceId, selectedTraceIndex]);

  return (
    <div
      className="overflow-hidden rounded-lg border border-border/60 bg-card dark:bg-surface"
    >
      {/* Filter bar: search on the left, status filter and the trace count on the right. */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 px-4 py-3">
        <FilterInput
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search name, ID, or user"
          aria-label="Search traces"
          containerClassName="h-8 w-64 max-w-full text-body-sm"
        />
        <div className="flex flex-wrap items-center gap-3">
          <MultiSelect value={selectedStatuses} onValueChange={setSelectedStatuses}>
            <MultiSelectTrigger className="h-8 w-44 max-w-full text-body-sm">
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
          <span className="shrink-0 text-mono-sm text-muted-foreground">
            {filtered.length} trace{filtered.length !== 1 ? "s" : ""}
          </span>
        </div>
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
          <div className="overflow-x-auto overscroll-x-contain [scrollbar-color:var(--border)_transparent] [scrollbar-width:thin] [&::-webkit-scrollbar]:h-2 [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-border [&::-webkit-scrollbar-track]:bg-transparent">
            <table className="w-full min-w-[52rem] table-fixed border-collapse">
              <colgroup>
                <col className="w-[18%]" />
                <col className="w-[13%]" />
                <col className="w-[24%]" />
                <col className="w-[11%]" />
                <col className="w-[12%]" />
                <col className="w-[22%]" />
              </colgroup>
              <thead>
                <tr className="border-b border-border/60">
                  <th className="whitespace-nowrap px-3 py-2 text-left text-mono-sm font-normal text-muted-foreground">Date</th>
                  <th className="whitespace-nowrap px-3 py-2 text-left text-mono-sm font-normal text-muted-foreground">Status</th>
                  <th className="whitespace-nowrap py-2 pl-3 pr-4 text-left text-mono-sm font-normal text-muted-foreground">User</th>
                  <th className="whitespace-nowrap py-2 pl-3 pr-2 text-left text-mono-sm font-normal text-muted-foreground">Latency</th>
                  <th className="whitespace-nowrap px-2 py-2 text-left text-mono-sm font-normal text-muted-foreground">Cost</th>
                  <th className="whitespace-nowrap px-3 py-2 text-mono-sm font-normal text-muted-foreground">
                    <span className="flex items-center justify-end gap-2">
                      <span className="block w-[11ch] text-left">Trace ID</span>
                      <span aria-hidden className="size-6 shrink-0" />
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((trace) => {
                  const isSelected = trace.trace_id === selectedTraceId;
                  return (
                    <tr
                      key={trace.trace_id}
                      id={traceRowAnchorId(trace.trace_id)}
                      onClick={() => onSelectTrace?.(trace)}
                      data-selected={isSelected || undefined}
                      className={cn(TRACE_ROW_CLASS, isSelected ? TRACE_ROW_SELECTED : TRACE_ROW_HOVER)}
                    >
                      <TraceRowCells trace={trace} account={account} />
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Pull the next server page when one exists (the endpoint caps a
              request at 100 traces). */}
          {hasMore && (
            <TableShowMore
              hiddenCount={1}
              expanded={false}
              onToggle={() => {
                if (!loadingMore) onLoadMore?.();
              }}
              showMoreLabel={loadingMore ? "Loading…" : "Load more"}
            />
          )}

        </>
      )}
    </div>
  );
}
