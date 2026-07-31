import { useState, useRef, useEffect, useLayoutEffect, useCallback, useDeferredValue } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import {
  AlertCircle,
  ArrowDown,
  Loader2,
  Pause,
  Play,
  RefreshCw,
  TriangleAlert,
  Copy,
  Check,
  Search,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  formatLogTimestamp,
  formatLogTimestampCompact,
  levelColorClass,
  normalizeLevel,
  type LogEntry,
} from "@/lib/log-utils";
import { useLogFiltering } from "@/hooks/use-log-filtering";
import { useDeploymentLogs } from "@/api/queries/deployments";
import { useLogTimezone } from "@/lib/timezone";
import { LogStreamProvider, useLogStream } from "@/lib/log-stream";
import type { WorkloadDetail } from "@/lib/api";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type TimeRange = "15m" | "1h" | "6h" | "24h" | "7d";

interface PrependAnchor {
  key: number;
  offset: number;
  previousCount: number;
}

const TIME_RANGE_OPTIONS: { value: TimeRange; label: string }[] = [
  { value: "15m", label: "15m" },
  { value: "1h", label: "1h" },
  { value: "6h", label: "6h" },
  { value: "24h", label: "24h" },
  { value: "7d", label: "7d" },
];

// ---------------------------------------------------------------------------
// Public component — wraps content in stream provider
// ---------------------------------------------------------------------------

interface PodLogsTabProps {
  workload: WorkloadDetail;
  deploymentId: string;
}

export function PodLogsTab({ workload, deploymentId }: PodLogsTabProps) {
  return (
    <LogStreamProvider>
      <PodLogsTabContent workload={workload} deploymentId={deploymentId} />
    </LogStreamProvider>
  );
}

// ---------------------------------------------------------------------------
// Inner content
// ---------------------------------------------------------------------------

function PodLogsTabContent({ workload, deploymentId }: PodLogsTabProps) {
  const containers = workload.containers ?? [];
  const [activeContainer, setActiveContainerRaw] = useState(containers[0]?.name ?? "");
  const [timeRange, setTimeRange] = useState<TimeRange>("1h");
  const [isTailing, setIsTailing] = useState(false);
  const [logSearch, setLogSearch] = useState("");
  const deferredSearch = useDeferredValue(logSearch);

  const { timezone } = useLogTimezone();
  const stream = useLogStream();
  const streamRef = useRef(stream);
  streamRef.current = stream;

  // Historical logs
  const {
    data: historicalLogs,
    isLoading,
    isRefetching,
    refetch,
    loadMore,
    isLoadingMore,
    hasMore,
  } = useDeploymentLogs(deploymentId, workload.name, activeContainer, timeRange, timezone, {
    enabled: !isTailing,
  });

  const logs = isTailing ? stream.lines : (historicalLogs ?? []);

  // Filtering
  const { activeFilters, toggleFilter, errCount, warnCount, filtered } =
    useLogFiltering(logs);

  // Search
  const searched = deferredSearch
    ? filtered.filter((e) =>
        e.message.toLowerCase().includes(deferredSearch.toLowerCase()),
      )
    : filtered;

  // Live tail controls
  const startTail = useCallback(() => {
    setIsTailing(true);
    stream.startStream(deploymentId, workload.name, activeContainer, timezone);
  }, [deploymentId, workload.name, activeContainer, timezone, stream]);

  const stopTail = useCallback(() => {
    setIsTailing(false);
    stream.stopStream();
  }, [stream]);

  const toggleTail = useCallback(() => {
    if (isTailing) stopTail();
    else startTail();
  }, [isTailing, startTail, stopTail]);

  const setActiveContainer = useCallback((name: string) => {
    setActiveContainerRaw((prev) => {
      if (prev !== name && isTailing) stopTail();
      return name;
    });
  }, [isTailing, stopTail]);

  // Cleanup on unmount
  useEffect(() => {
    return () => streamRef.current.stopStream();
  }, []);

  // Copy
  const { copy, copied } = useCopyToClipboard(1200);
  const handleCopy = useCallback(() => {
    const text = searched
      .map((e) => `${e.timestamp ?? ""} ${e.level ?? ""} ${e.message}`.trim())
      .join("\n");
    void copy(text);
  }, [searched, copy]);

  return (
    <div className="flex h-full flex-col gap-2">
      {/* Container switcher (only if multiple) */}
      {containers.length > 1 && (
        <div className="flex items-center gap-2">
          <span className="text-mono-sm font-medium text-muted-foreground dark:text-white/25">Container</span>
          <div className="flex items-center overflow-hidden rounded border border-border dark:border-white/6">
            {containers.map((c) => (
              <button
                key={c.name}
                onClick={() => setActiveContainer(c.name)}
                className={cn(
                  "px-2.5 py-1 text-mono-sm transition-colors",
                  activeContainer === c.name
                    ? "bg-black/6 text-foreground dark:bg-white/10 dark:text-white/80"
                    : "text-muted-foreground hover:text-foreground/70 dark:text-white/30 dark:hover:text-white/50",
                )}
              >
                {c.name}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Toolbar */}
      <div className="@container/toolbar flex flex-col gap-1.5">
        <div className="flex flex-wrap items-center gap-1.5">
          {/* Error / Warning filters */}
          <FilterButton
            count={errCount}
            active={activeFilters.has("errors")}
            colorClass="text-coral-600"
            icon={<AlertCircle className="size-3" />}
            onClick={() => toggleFilter("errors")}
          />
          <FilterButton
            count={warnCount}
            active={activeFilters.has("warnings")}
            colorClass="text-yellow-600"
            icon={<TriangleAlert className="size-3" />}
            onClick={() => toggleFilter("warnings")}
          />

          {/* Search — inline at wide, moves to own row at narrow */}
          <div className="flex flex-1 items-center gap-1.5 rounded border border-input bg-[color-mix(in_oklch,var(--input-background)_45%,transparent)] px-2 py-1 @max-[575px]/toolbar:order-last @max-[575px]/toolbar:basis-full">
            <Search className="size-3 shrink-0 text-faint-foreground dark:text-faint-foreground/70" />
            <input
              type="text"
              placeholder="Search"
              value={logSearch}
              onChange={(e) => setLogSearch(e.target.value)}
              className="w-full min-w-0 border-none bg-transparent text-mono-sm text-foreground/70 outline-none placeholder:text-faint-foreground dark:text-faint-foreground dark:placeholder:text-faint-foreground/70"
            />
          </div>

          {/* Time range */}
          <div className="flex items-center overflow-hidden rounded border border-border dark:border-white/6">
            {TIME_RANGE_OPTIONS.map((o) => (
              <button
                key={o.value}
                onClick={() => setTimeRange(o.value)}
                disabled={isTailing}
                className={cn(
                  "px-2 py-1 text-mono-sm transition-colors disabled:opacity-30",
                  timeRange === o.value && !isTailing
                    ? "bg-black/6 text-foreground dark:bg-white/10 dark:text-white/80"
                    : "text-muted-foreground hover:text-foreground/70 dark:text-white/30 dark:hover:text-white/50",
                )}
              >
                {o.label}
              </button>
            ))}
          </div>

          {/* Live tail */}
          <button
            onClick={toggleTail}
            className={cn(
              "flex items-center gap-1 rounded border px-2 py-1 text-mono-sm transition-colors",
              isTailing
                ? "border-teal-500/30 bg-teal-500/15 text-teal-600 dark:border-white/6 dark:text-teal-400"
                : "border-border text-muted-foreground hover:text-foreground/70 dark:border-white/6 dark:text-white/30 dark:hover:text-white/50",
            )}
          >
            {isTailing ? <Pause className="size-3" /> : <Play className="size-3" />}
            Live
          </button>

          {/* Refresh */}
          <button
            onClick={() => void refetch()}
            disabled={isTailing}
            className="rounded border border-border p-1 text-muted-foreground transition-colors hover:text-foreground/70 disabled:opacity-30 dark:border-white/6 dark:text-white/30 dark:hover:text-white/50"
          >
            <RefreshCw className={cn("size-3", isRefetching && "animate-spin")} />
          </button>

          {/* Copy */}
          <button
            onClick={handleCopy}
            className="rounded border border-border p-1 text-muted-foreground transition-colors hover:text-foreground/70 dark:border-white/6 dark:text-white/30 dark:hover:text-white/50"
          >
            {copied ? (
              <Check className="size-3 text-teal-400" />
            ) : (
              <Copy className="size-3" />
            )}
          </button>
        </div>
      </div>

      {/* Log output */}
      <LogOutput
        logs={searched}
        isLoading={isLoading}
        isTailing={isTailing}
        isReconnecting={stream.status === "reconnecting"}
        error={stream.error}
        search={deferredSearch}
        onLoadMore={isTailing ? undefined : loadMore}
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Filter button
// ---------------------------------------------------------------------------

function FilterButton({
  count,
  active,
  colorClass,
  icon,
  onClick,
}: {
  count: number;
  active: boolean;
  colorClass: string;
  icon: React.ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        "group flex items-center gap-1 rounded border border-border px-2 py-1 text-mono-sm transition-colors dark:border-white/6",
        colorClass,
        active ? "bg-black/4 dark:bg-white/6" : "hover:bg-muted/40",
      )}
    >
      <span className={cn(!active && "opacity-60 transition-opacity group-hover:opacity-80")}>
        {icon}
      </span>
      <span className={cn(!active && "opacity-60 transition-opacity group-hover:opacity-80")}>
        {count}
      </span>
    </button>
  );
}

// ---------------------------------------------------------------------------
// Log output (virtualized)
// ---------------------------------------------------------------------------

function LogOutput({
  logs,
  isLoading,
  isTailing,
  isReconnecting,
  error,
  search,
  onLoadMore,
  isLoadingMore,
  hasMore,
}: {
  logs: LogEntry[];
  isLoading: boolean;
  isTailing: boolean;
  isReconnecting: boolean;
  error?: string;
  search: string;
  onLoadMore?: () => Promise<unknown>;
  isLoadingMore?: boolean;
  hasMore?: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const isUserScrolled = useRef(false);
  const [showJump, setShowJump] = useState(false);

  // Capture the visible row before older pages prepend. Restoring by row
  // identity avoids scroll-height drift while virtual rows are remeasured.
  const prependAnchorRef = useRef<PrependAnchor | null>(null);

  const seqMapRef = useRef<WeakMap<LogEntry, number>>(new WeakMap());
  const seqCounterRef = useRef(0);
  const getLogKey = useCallback((entry: LogEntry) => {
    let seq = seqMapRef.current.get(entry);
    if (seq === undefined) {
      seq = seqCounterRef.current++;
      seqMapRef.current.set(entry, seq);
    }
    return seq;
  }, []);

  // useVirtualizer returns non-memoizable functions; React Compiler will
  // automatically skip memoizing this component. Disable the rule explicitly
  // so the lint output stays clean.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: logs.length,
    getScrollElement: () => scrollRef.current,
    getItemKey: (index) => getLogKey(logs[index]!),
    estimateSize: () => 24,
    overscan: 15,
  });

  const scrollToBottom = useCallback(() => {
    if (logs.length > 0) {
      virtualizer.scrollToIndex(logs.length - 1, { align: "end" });
    }
    isUserScrolled.current = false;
    setShowJump(false);
  }, [logs.length, virtualizer]);

  const capturePrependAnchor = useCallback(() => {
    const element = scrollRef.current;
    if (!element) return null;

    const anchorItem = virtualizer
      .getVirtualItems()
      .find((item) => item.end > element.scrollTop);
    if (!anchorItem) return null;

    const entry = logs[anchorItem.index];
    if (!entry) return null;

    return {
      key: getLogKey(entry),
      offset: anchorItem.start - element.scrollTop,
      previousCount: logs.length,
    };
  }, [getLogKey, logs, virtualizer]);

  // Once the query settles, restore the same visible log when matching rows
  // were prepended. Filtered and empty pages simply clear the anchor.
  useLayoutEffect(() => {
    const anchor = prependAnchorRef.current;
    if (!anchor || isLoadingMore) return;

    prependAnchorRef.current = null;
    if (logs.length > anchor.previousCount) {
      const index = logs.findIndex((entry) => getLogKey(entry) === anchor.key);
      const offsetInfo = index >= 0
        ? virtualizer.getOffsetForIndex(index, "start")
        : undefined;
      if (offsetInfo) {
        virtualizer.scrollToOffset(
          Math.max(0, offsetInfo[0] - anchor.offset),
          { align: "start" },
        );
      }
    }
  }, [getLogKey, hasMore, isLoadingMore, logs, virtualizer]);

  const handleLoadMore = useCallback(() => {
    if (!onLoadMore || isLoadingMore) return;

    prependAnchorRef.current = capturePrependAnchor();
    isUserScrolled.current = true;
    void onLoadMore();
  }, [capturePrependAnchor, isLoadingMore, onLoadMore]);

  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
    const scrolledUp = dist > 60;
    isUserScrolled.current = scrolledUp;
    setShowJump(scrolledUp);
  }, []);

  // Auto-scroll on new logs
  useEffect(() => {
    if (!isUserScrolled.current) scrollToBottom();
  }, [logs.length, scrollToBottom]);

  // Force scroll when entering tail mode
  useEffect(() => {
    if (isTailing) {
      isUserScrolled.current = false;
      setShowJump(false);
      scrollToBottom();
    }
  }, [isTailing, scrollToBottom]);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-4 text-mono-sm text-muted-foreground dark:text-white/30">
        <Loader2 className="size-3.5 animate-spin" />
        Loading logs…
      </div>
    );
  }

  if (isReconnecting) {
    return (
      <div className="flex items-center gap-2 py-4 text-mono-sm text-muted-foreground dark:text-white/30">
        <Loader2 className="size-3.5 animate-spin" />
        Reconnecting…
      </div>
    );
  }

  if (error) {
    return <p className="py-4 text-mono-sm text-red-400">{error}</p>;
  }

  const canLoadMore = Boolean(onLoadMore && (hasMore || isLoadingMore));

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-1.5">
      {canLoadMore && (
        <Button
          type="button"
          variant="outline"
          size="xs"
          onClick={handleLoadMore}
          disabled={isLoadingMore}
          className="self-center font-mono text-mono-sm text-muted-foreground hover:text-foreground"
        >
          {isLoadingMore && <Loader2 className="size-3 animate-spin" />}
          {isLoadingMore ? "Loading older logs…" : "Load older logs"}
        </Button>
      )}
      {logs.length === 0 ? (
        <p className="py-4 text-mono-sm text-muted-foreground/60 dark:text-white/20">
          {isTailing ? "Waiting for logs…" : "No logs in this time window"}
        </p>
      ) : (
        <div className="relative min-h-0 flex-1">
          <div
            ref={scrollRef}
            onScroll={handleScroll}
            className="h-full overflow-y-auto rounded border border-border/60 bg-black/2 py-1 dark:border-white/4 dark:bg-black/20"
          >
            <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
              {virtualizer.getVirtualItems().map((vItem) => {
                const entry = logs[vItem.index];
                const level = normalizeLevel(entry.level);
                const lvlClass = levelColorClass(entry.level);
                return (
                  <div
                    key={vItem.key}
                    data-index={vItem.index}
                    ref={virtualizer.measureElement}
                    className="flex items-baseline gap-x-2 px-3 py-0.5 font-mono text-mono-sm leading-5"
                    style={{
                      position: "absolute",
                      top: 0,
                      left: 0,
                      width: "100%",
                      transform: `translateY(${vItem.start}px)`,
                    }}
                  >
                    <span className="w-[16ch] shrink-0 text-muted-foreground dark:text-white/20" title={formatLogTimestamp(entry.timestamp)}>
                      {formatLogTimestampCompact(entry.timestamp)}
                    </span>
                    <span className={cn("w-[5ch] shrink-0 font-medium", lvlClass)}>
                      {level}
                    </span>
                    <span className="text-foreground/70 dark:text-white/60">
                      {search ? highlightSearch(entry.message, search) : entry.message}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
          {showJump && (
            <button
              onClick={scrollToBottom}
              className="absolute bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-1 rounded-full border border-border bg-white/80 px-3 py-1 text-mono-sm text-muted-foreground backdrop-blur-sm transition-colors hover:text-foreground dark:border-white/8 dark:bg-black/60 dark:text-white/50 dark:hover:text-white/70"
            >
              <ArrowDown className="size-3" />
              Jump to bottom
            </button>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Search highlighting
// ---------------------------------------------------------------------------

function highlightSearch(text: string, search: string): React.ReactNode {
  if (!search) return text;
  const lower = text.toLowerCase();
  const lowerSearch = search.toLowerCase();
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  let idx = lower.indexOf(lowerSearch, cursor);
  while (idx !== -1) {
    if (idx > cursor) parts.push(text.slice(cursor, idx));
    parts.push(
      <mark key={idx} className="rounded-[2px] bg-yellow-300/40 text-inherit">
        {text.slice(idx, idx + search.length)}
      </mark>,
    );
    cursor = idx + search.length;
    idx = lower.indexOf(lowerSearch, cursor);
  }
  if (cursor < text.length) parts.push(text.slice(cursor));
  return <>{parts}</>;
}
