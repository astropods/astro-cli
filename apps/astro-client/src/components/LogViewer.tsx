import { useState, useRef, useEffect, useCallback } from "react";
import { AlertCircle, ArrowDown, Loader2, Pause, Play, TriangleAlert, X } from "lucide-react";
import { MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
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

const TailDot = () => <span className="size-1.5 rounded-full bg-teal-500 shrink-0 dp-blink" />;

const FILTER_CONFIGS = [
  { key: "errors" as const, label: "Errors", icon: AlertCircle, colorClass: "text-coral-600" },
  { key: "warnings" as const, label: "Warnings", icon: TriangleAlert, colorClass: "text-yellow-600" },
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
  const filterCounts = { errors: errCount, warnings: warnCount };

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

  useEffect(() => {
    if (isTailing) {
      isUserScrolled.current = false;
      setShowJumpToBottom(false);
      scrollToBottom();
    }
  }, [isTailing, scrollToBottom]);

  useEffect(() => {
    if (!isUserScrolled.current) {
      scrollToBottom();
    }
  }, [logs.length, scrollToBottom]);

  return (
    <div className="flex flex-col h-full bg-surface border border-border rounded-[10px] overflow-hidden">

      {/* Toolbar */}
      <div className="flex items-center gap-2 px-4 py-[9px] border-b border-border flex-shrink-0 flex-wrap">
        <div className="flex items-center gap-2">
          {leading}
          {FILTER_CONFIGS.map((f) => {
            const active = activeFilters.has(f.key);
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
                {isCompact ? <f.icon size={12} className="shrink-0" /> : f.label}
                <span className="font-mono text-mono-sm">{filterCounts[f.key]}</span>
                {active && <X size={10} className="shrink-0" />}
              </Button>
            );
          })}
        </div>

        <div className={cn("flex items-center gap-2 flex-1 min-w-0", !isCompact && "justify-end")}>
          <div className={cn("flex items-center gap-[5px] h-8 px-2.5 rounded-[calc(var(--radius-sm)+2px)] border border-border bg-popover", isCompact ? "flex-1 min-w-0" : "shrink-0")}>
            <MagnifyingGlassIcon className="size-3 text-faint-foreground shrink-0" />
            <input
              type="text"
              placeholder="Search logs"
              value={logSearch}
              onChange={(e) => setLogSearch(e.target.value)}
              className={cn("bg-transparent border-none outline-none font-sans text-body-sm text-muted-foreground caret-teal-600", isCompact ? "w-full" : "w-60")}
            />
          </div>

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
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={onTailToggle}
                    className={cn(
                      "font-sans text-body-sm gap-[5px]",
                      isTailing ? "bg-muted font-medium" : "font-normal",
                    )}
                  >
                    {isTailing ? <Pause className="size-3 shrink-0" /> : <Play className="size-3 shrink-0" />}
                    Live tail
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{isTailing ? "Stop live tailing" : "Start live tailing"}</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}

          <CopyButton
            copyText={() => filtered.map((e) => `${e.timestamp ?? ""} ${e.level ?? ""} ${e.message}`.trim()).join("\n")}
            title="Copy logs"
            resetMs={900}
          />
        </div>
      </div>

      {/* Log stream */}
      <div className="flex flex-col flex-1 min-h-0">
        {isTailing && filtered.length > 0 && (
          <div className="flex items-center gap-2 px-[18px] pt-5 pb-3 font-mono text-mono-sm text-faint-foreground bg-background flex-shrink-0">
            <TailDot />
            Live tail active — new lines appear as they arrive
          </div>
        )}
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
            <div className="flex items-center gap-2 px-[18px] py-3 font-mono text-mono-sm text-faint-foreground">
              {isTailing && <TailDot />}
              {logs.length === 0
                ? isTailing ? "Waiting on live tail results…" : "No log lines in this time window"
                : "No matching lines"}
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
    </div>
  );
}
