import { useState, useRef, useEffect, useCallback } from "react";
import { Loader2, X, ArrowDown } from "lucide-react";
import { MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { formatLogTimestamp, levelColorClass, normalizeLevel, type LogEntry } from "@/lib/log-utils";
import { useLogFiltering } from "@/hooks/use-log-filtering";

export type LogTimeRange = "15m" | "1h" | "6h" | "24h" | "7d";

export const TIME_RANGE_OPTIONS: { value: LogTimeRange; label: string }[] = [
  { value: "15m", label: "Last 15 min" },
  { value: "1h", label: "Last 1 hour" },
  { value: "6h", label: "Last 6 hours" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
];

const FILTER_CONFIGS = [
  { key: "errors" as const, label: "Errors", colorClass: "text-coral-600" },
  { key: "warnings" as const, label: "Warnings", colorClass: "text-yellow-600" },
] as const;

interface LogViewerProps {
  logs: LogEntry[];
  isLoading?: boolean;
  isCompact?: boolean;
  timeRange: LogTimeRange;
  onTimeRangeChange: (range: LogTimeRange) => void;
  /** Optional content rendered at the left of the toolbar */
  leading?: React.ReactNode;
  error?: string;
  isTailing?: boolean;
  isReconnecting?: boolean;
  onTailToggle?: () => void;
}

export function LogViewer({ logs, isLoading = false, isCompact = false, timeRange, onTimeRangeChange, leading, error, isTailing = false, isReconnecting = false, onTailToggle }: LogViewerProps) {
  const [logSearch, setLogSearch] = useState("");
  const { activeFilters, toggleFilter, errCount, warnCount, filtered } = useLogFiltering(logs, logSearch);

  const scrollRef = useRef<HTMLDivElement>(null);
  const isUserScrolled = useRef(false);
  const [showJumpToBottom, setShowJumpToBottom] = useState(false);

  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    isUserScrolled.current = false;
    setShowJumpToBottom(false);
  }, []);

  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const distFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    const scrolledUp = distFromBottom > 80;
    isUserScrolled.current = scrolledUp;
    setShowJumpToBottom(scrolledUp);
  }, []);

  // Auto-scroll when new log data arrives (if user is at bottom)
  useEffect(() => {
    if (!isUserScrolled.current) {
      scrollToBottom();
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [logs.length]);

  return (
    <div className="flex flex-col h-full bg-surface border border-border rounded-[10px] overflow-hidden">

      {/* Toolbar */}
      <div className="flex items-center gap-2 px-4 py-[9px] border-b border-border flex-shrink-0 flex-wrap">
        {leading}
        <div className="flex-1" />

        {/* Level filters */}
        {FILTER_CONFIGS.map((f) => {
          const active = activeFilters.has(f.key);
          const count = f.key === "errors" ? errCount : warnCount;
          return (
            <Button
              key={f.key}
              variant="outline"
              size="sm"
              onClick={() => toggleFilter(f.key)}
              className={cn(
                "font-sans text-body-sm gap-[5px]",
                f.colorClass,
                active ? "bg-muted font-medium" : "font-normal",
              )}
            >
              {f.label}
              <span className="font-mono text-mono-sm">{count}</span>
              {active && <X size={10} className="shrink-0" />}
            </Button>
          );
        })}

        {/* Search */}
        <div className="flex items-center gap-[5px] h-8 px-2.5 rounded-[calc(var(--radius-sm)+2px)] border border-border bg-popover">
          <MagnifyingGlassIcon className="size-3 text-faint-foreground shrink-0" />
          <input
            type="text"
            placeholder="Search logs"
            value={logSearch}
            onChange={(e) => setLogSearch(e.target.value)}
            className={cn(
              "bg-transparent border-none outline-none font-sans text-body-sm text-muted-foreground caret-teal-600",
              isCompact ? "w-[92px]" : "w-40",
            )}
          />
        </div>

        {/* Time range + Live toggle */}
        <Select
          value={timeRange}
          disabled={isTailing}
          onValueChange={(v) => onTimeRangeChange(v as LogTimeRange)}
        >
          <SelectTrigger className="h-8 w-auto min-w-[130px] px-3 font-sans text-body-sm bg-popover rounded-[calc(var(--radius-sm)+2px)] disabled:pointer-events-auto disabled:cursor-not-allowed">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_RANGE_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {onTailToggle && (
          <Button
            variant="outline"
            size="sm"
            onClick={onTailToggle}
            className={cn(
              "font-sans text-body-sm gap-[5px]",
              isTailing ? "bg-muted font-medium" : "font-normal",
            )}
          >
            <span
              className={cn(
                "size-1.5 rounded-full shrink-0",
                isTailing ? "bg-teal-500 animate-pulse" : "bg-muted-foreground",
              )}
            />
            Tail
          </Button>
        )}

        <CopyButton
          copyText={() => filtered.map((e) => `${e.timestamp ?? ""} ${e.level ?? ""} ${e.message}`.trim()).join("\n")}
          title="Copy logs"
          resetMs={900}
        />
      </div>

      {/* Log stream */}
      <div className="relative flex-1 min-h-0">
      <div ref={scrollRef} onScroll={handleScroll} className="h-full overflow-y-auto bg-background py-2.5 pb-3.5">
        {isLoading ? (
          <div className="flex items-center gap-2 px-[18px] py-3 font-mono text-mono-sm text-faint-foreground">
            <Loader2 size={14} className="dp-spin" />
            Loading logs…
          </div>
        ) : isReconnecting ? (
          <div className="flex items-center gap-2 px-[18px] py-3 font-mono text-mono-sm text-faint-foreground">
            <Loader2 size={14} className="dp-spin" />
            Reconnecting…
          </div>
        ) : error ? (
          <div className="px-[18px] py-3 font-mono text-mono-sm text-destructive">
            {error}
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-[18px] py-3 font-mono text-mono-sm text-faint-foreground">
            {logs.length === 0 ? "No log lines in this time window" : "No matching lines"}
          </div>
        ) : (
          filtered.map((entry, li) => {
            const level = normalizeLevel(entry.level);
            const lvlClass = levelColorClass(entry.level);
            return (
              <div key={li} className="dp-log flex items-baseline gap-x-3 px-[18px] py-1 font-mono text-mono-sm tracking-normal leading-5">
                <span className="text-faint-foreground shrink-0 w-[24ch]">
                  {formatLogTimestamp(entry.timestamp)}
                </span>
                <span className={cn("font-medium w-[5ch] shrink-0", lvlClass)}>
                  {level}
                </span>
                <span className="text-foreground whitespace-nowrap">
                  {entry.message}
                </span>
              </div>
            );
          })
        )}
      </div>
      {showJumpToBottom && (
        <Button
          variant="outline"
          size="sm"
          onClick={scrollToBottom}
          className="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full bg-surface shadow-sm font-sans text-body-sm text-muted-foreground hover:text-foreground gap-1.5"
        >
          <ArrowDown size={12} />
          Jump to bottom
        </Button>
      )}
      </div>
    </div>
  );
}
