import { useEffect, useMemo, useState } from "react";
import { Check, ChevronDown, Loader2, User, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { CopyButton } from "@/components/ui/copy-button";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { FilterInput } from "@/components/FilterInput";
import { UserAvatar } from "@/components/UserAvatar";
import { TableShowMore } from "@/components/ui/table";
import { traceRowAnchorId } from "@/lib/routes";
import type { TraceEntry, TraceUserFacet } from "@/lib/api";
import {
  slackIdentityDisplay,
} from "@/components/activity/insights-user-identity";
import { TraceUserIdentity } from "./TraceUserIdentity";
import {
  formatTimestamp,
  formatLatency,
  formatCost,
} from "./trace-utils";

const TRACE_ROW_CLASS = "group cursor-pointer border-b border-border/30 transition-colors";
const TRACE_ROW_SELECTED = "bg-black/3 dark:bg-white/4";
const TRACE_ROW_HOVER = "hover:bg-black/2 dark:hover:bg-white/3";
const NO_USER_KEY = "__no_user__";

export function traceUserFilterParams(selectedKey: string | null): Record<string, string> {
  if (selectedKey === NO_USER_KEY) return { no_user: "true" };
  if (selectedKey?.startsWith("user:")) {
    return { user_id: selectedKey.slice("user:".length) };
  }
  return {};
}

export type TraceSortKey = "timestamp" | "latency" | "cost";
type TraceSortDirection = "asc" | "desc";
export type TraceSortState = { key: TraceSortKey; direction: TraceSortDirection };

interface TraceUserFilterOption {
  key: string;
  label: string;
  avatarUrl?: string;
  avatarHandle?: string;
  count: number;
}

function nextTraceSort(
  current: TraceSortState,
  key: TraceSortKey,
): TraceSortState {
  if (current.key === key) {
    return { key, direction: current.direction === "asc" ? "desc" : "asc" };
  }
  return { key, direction: "desc" };
}

function buildTraceUserOptions(
  facets: TraceUserFacet[],
) {
  const options: TraceUserFilterOption[] = [];
  for (const facet of facets) {
    const key = facet.user_id ? `user:${facet.user_id}` : NO_USER_KEY;
    if (!facet.user_id) {
      options.push({
        key,
        label: "No user",
        count: facet.count,
      });
      continue;
    }
    const details = facet.user_details;
    const slackDisplay = details?.kind === "slack"
      ? slackIdentityDisplay({ user_id: facet.user_id, user_details: details })
      : null;
    const label = slackDisplay?.primary
      || details?.display_name?.trim()
      || details?.username?.trim()
      || facet.user_id;
    options.push({
      key,
      label,
      avatarUrl: details?.avatar_url,
      avatarHandle: details?.kind === "astro"
        ? details?.username?.trim()
        : undefined,
      count: facet.count,
    });
  }
  return options.sort((a, b) => {
    if (a.key === NO_USER_KEY) return 1;
    if (b.key === NO_USER_KEY) return -1;
    return a.label.localeCompare(b.label);
  });
}

function UserFilterHead({
  options,
  selectedKey,
  onSelect,
}: {
  options: TraceUserFilterOption[];
  selectedKey: string | null;
  onSelect: (key: string | null) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const filteredOptions = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return normalizedQuery
      ? options.filter((option) =>
          `${option.label} ${option.key}`.toLowerCase().includes(normalizedQuery),
        )
      : options;
  }, [options, query]);
  const choose = (key: string | null) => {
    onSelect(key);
    setOpen(false);
  };

  return (
    <th className="whitespace-nowrap py-2 pl-3 pr-4 text-left text-mono-sm font-normal text-muted-foreground">
      <Popover
        open={open}
        onOpenChange={(nextOpen) => {
          setOpen(nextOpen);
          if (!nextOpen) setQuery("");
        }}
      >
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className={cn(
              "-mx-2 h-7 text-mono-sm font-normal text-muted-foreground hover:text-foreground",
              selectedKey && "text-foreground",
            )}
            aria-label="Filter by user"
          >
            User <ChevronDown aria-hidden className="size-3.5" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          side="bottom"
          align="start"
          sideOffset={8}
          className="w-80 overflow-hidden rounded-md border border-border bg-popover p-0 text-popover-foreground shadow-lg dark:bg-popover dark:text-popover-foreground"
        >
            <div className="flex items-center justify-between border-b border-border px-3 py-2">
              <span className="text-body-sm font-semibold text-foreground">Filter by user</span>
              <Button
                type="button"
                variant="ghost"
                size="icon-xs"
                className="text-muted-foreground hover:bg-muted hover:text-foreground"
                aria-label="Close user filter"
                onClick={() => setOpen(false)}
              >
                <X aria-hidden className="size-4" />
              </Button>
            </div>
            <div className="border-b border-border p-2">
              <FilterInput
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Filter users"
                containerClassName="h-9 w-full"
                className="text-body-sm"
                autoFocus
              />
              <p className="px-1 pt-2 text-mono-xs text-muted-foreground">
                Users and counts reflect the selected window.
              </p>
            </div>
            <div className="max-h-72 overflow-y-auto py-1">
              <Button
                type="button"
                variant="ghost"
                className={cn(
                  "h-auto w-full justify-start rounded-none px-3 py-2 text-left text-body-sm font-normal hover:bg-muted/60 dark:hover:bg-white/10",
                  !selectedKey && "bg-muted/40 dark:bg-white/5",
                )}
                onClick={() => choose(null)}
              >
                <span className="flex size-4 items-center justify-center">
                  {!selectedKey && <Check aria-hidden className="size-3.5" />}
                </span>
                <span className="font-medium text-foreground">All users</span>
              </Button>
              {filteredOptions.length === 0 ? (
                <div className="px-3 py-6 text-center text-body-sm text-muted-foreground">
                  No users found.
                </div>
              ) : filteredOptions.map((option) => (
                <Button
                  key={option.key}
                  type="button"
                  variant="ghost"
                  className={cn(
                    "h-auto w-full justify-start rounded-none px-3 py-2 text-left font-normal hover:bg-muted/60 dark:hover:bg-white/10",
                    option.key === selectedKey && "bg-muted/40 dark:bg-white/5",
                  )}
                  onClick={() => choose(option.key)}
                >
                  <span className="flex size-4 items-center justify-center">
                    {option.key === selectedKey && <Check aria-hidden className="size-3.5" />}
                  </span>
                  {option.avatarHandle || option.avatarUrl ? (
                    <UserAvatar
                      handle={option.avatarHandle}
                      name={option.label}
                      avatarUrl={option.avatarUrl}
                      className="size-6"
                    />
                  ) : (
                    <span className="flex size-6 items-center justify-center rounded-full bg-muted text-muted-foreground">
                      <User aria-hidden className="size-3.5" />
                    </span>
                  )}
                  <span className="min-w-0 flex-1 truncate text-body-sm font-medium text-foreground">
                    {option.label}
                  </span>
                  <span className="text-mono-sm text-muted-foreground">{option.count}</span>
                </Button>
              ))}
            </div>
        </PopoverContent>
      </Popover>
    </th>
  );
}

function SortableHead({
  label,
  sortKey,
  sort,
  onSort,
  className,
}: {
  label: string;
  sortKey: TraceSortKey;
  sort: TraceSortState;
  onSort: (key: TraceSortKey) => void;
  className?: string;
}) {
  const active = sort.key === sortKey;
  return (
    <th
      className={cn("text-left text-mono-sm font-normal text-muted-foreground", className)}
      aria-sort={active ? (sort.direction === "asc" ? "ascending" : "descending") : "none"}
    >
      <button
        type="button"
        className="inline-flex h-7 items-center gap-1.5 rounded-sm hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
        onClick={() => onSort(sortKey)}
        aria-label={`Sort by ${label.toLowerCase()}`}
      >
        {label}
        {active && <span aria-hidden className="text-foreground">{sort.direction === "asc" ? "↑" : "↓"}</span>}
      </button>
    </th>
  );
}

function shortTraceId(traceId: string) {
  if (traceId.length <= 16) return traceId;
  return `...${traceId.slice(-8)}`;
}

function TraceRowCells({ trace, account }: { trace: TraceEntry; account: string }) {
  const timestamp = formatTimestamp(trace.timestamp);
  return (
    <>
      <td className="truncate whitespace-nowrap px-3 py-2.5 text-body-sm text-foreground" title={timestamp}>
        {timestamp}
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
  /** Complete user facets for the selected deployment window. */
  userFacets: TraceUserFacet[];
  /** Account that owns the deployment — used to resolve trace user IDs to profiles. */
  account: string;
  search: string;
  onSearchChange: (search: string) => void;
  selectedUserKey: string | null;
  onSelectedUserKeyChange: (key: string | null) => void;
  sort: TraceSortState;
  onSortChange: (sort: TraceSortState) => void;
  loading?: boolean;
  selectedTraceId?: string | null;
  onSelectTrace?: (trace: TraceEntry) => void;
  /** Another page of traces exists on the server past the loaded window. */
  hasMore?: boolean;
  /** Fetch the next server page; appended to the loaded window. */
  onLoadMore?: () => void;
  /** Collapse the table back to its initial page. */
  onShowLess?: () => void;
  /** Rows currently shown beyond the initial page. */
  revealedCount?: number;
  /** The next page is in flight. */
  loadingMore?: boolean;
  /** Total server-filtered traces in the selected window. */
  totalCount?: number;
  /** Traces fetched from the server, including rows hidden by Show less. */
  loadedCount?: number;
  /** The server capped an Astro-side criteria candidate set. */
  resultsTruncated?: boolean;
  /** Number of candidate records inspected before the cap. */
  scannedCount?: number;
}

export function TracesTable({
  traces,
  userFacets,
  account,
  search,
  onSearchChange,
  selectedUserKey,
  onSelectedUserKeyChange,
  sort,
  onSortChange,
  loading,
  selectedTraceId,
  onSelectTrace,
  hasMore,
  onLoadMore,
  onShowLess,
  revealedCount = 0,
  loadingMore,
  totalCount = traces.length,
  loadedCount = traces.length,
  resultsTruncated = false,
  scannedCount = 0,
}: TracesTableProps) {
  const userOptions = useMemo(
    () => buildTraceUserOptions(userFacets),
    [userFacets],
  );

  const selectedTraceIndex = selectedTraceId
    ? traces.findIndex((trace) => trace.trace_id === selectedTraceId)
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
      {/* Filter bar: search on the left and the server-filtered trace count on the right. */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 px-4 py-3">
        <FilterInput
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="Search name, ID, or user"
          aria-label="Search traces"
          containerClassName="h-8 w-64 max-w-full text-body-sm"
        />
        <div className="flex flex-wrap items-center gap-3">
          {resultsTruncated && (
            <span className="shrink-0 text-mono-sm text-warning">
              Partial results{scannedCount > 0 ? ` · ${scannedCount.toLocaleString()} candidates checked` : ""}
            </span>
          )}
          <span className="shrink-0 text-mono-sm text-muted-foreground">
            {traces.length < totalCount
              ? `${traces.length} shown · ${loadedCount} of ${totalCount} loaded`
              : `${traces.length} trace${traces.length !== 1 ? "s" : ""}`}
          </span>
        </div>
      </div>

      {loading ? (
        <div className="flex h-[200px] items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : traces.length === 0 ? (
        <div className="flex h-[200px] items-center justify-center">
          <p className="text-body-sm text-muted-foreground">No traces found.</p>
        </div>
      ) : (
        <>
          <div className="overflow-x-auto overscroll-x-contain [scrollbar-color:var(--border)_transparent] [scrollbar-width:thin] [&::-webkit-scrollbar]:h-2 [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-border [&::-webkit-scrollbar-track]:bg-transparent">
            <table className="w-full min-w-[52rem] table-fixed border-collapse">
              <colgroup>
                <col className="w-[20%]" />
                <col className="w-[29%]" />
                <col className="w-[12%]" />
                <col className="w-[13%]" />
                <col className="w-[26%]" />
              </colgroup>
              <thead>
                <tr className="border-b border-border/60">
                  <SortableHead
                    label="Date"
                    sortKey="timestamp"
                    sort={sort}
                    onSort={(key) => onSortChange(nextTraceSort(sort, key))}
                    className="whitespace-nowrap px-3 py-2"
                  />
                  <UserFilterHead
                    options={userOptions}
                    selectedKey={selectedUserKey}
                    onSelect={onSelectedUserKeyChange}
                  />
                  <SortableHead
                    label="Latency"
                    sortKey="latency"
                    sort={sort}
                    onSort={(key) => onSortChange(nextTraceSort(sort, key))}
                    className="whitespace-nowrap py-2 pl-3 pr-2"
                  />
                  <SortableHead
                    label="Cost"
                    sortKey="cost"
                    sort={sort}
                    onSort={(key) => onSortChange(nextTraceSort(sort, key))}
                    className="whitespace-nowrap px-2 py-2"
                  />
                  <th className="whitespace-nowrap px-3 py-2 text-mono-sm font-normal text-muted-foreground">
                    <span className="flex items-center justify-end gap-2">
                      <span className="block w-[11ch] text-left">Trace ID</span>
                      <span aria-hidden className="size-6 shrink-0" />
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {traces.map((trace) => {
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

          {(hasMore || revealedCount > 0) && (
            <TableShowMore
              hiddenCount={hasMore ? Math.max(totalCount - traces.length, 1) : 0}
              expanded={revealedCount > 0}
              revealedCount={revealedCount}
              onToggle={() => {
                if (!loadingMore) onLoadMore?.();
              }}
              onShowLess={onShowLess}
              showMoreLabel={loadingMore ? "Loading…" : "Show more"}
            />
          )}
        </>
      )}
    </div>
  );
}
