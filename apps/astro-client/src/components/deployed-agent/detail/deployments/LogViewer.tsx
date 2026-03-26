import { Search, Loader2, X, RefreshCw, Copy, Check } from "lucide-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { logLineColorClass, splitLogLineTimestamp, formatLogTimestamp } from "@/lib/log-utils";

export type LogTimeRange = "15m" | "1h" | "6h" | "24h" | "7d";

export const LOG_TIME_RANGE_OPTIONS: { value: LogTimeRange; label: string }[] = [
  { value: "15m", label: "Last 15 min" },
  { value: "1h", label: "Last 1 hour" },
  { value: "6h", label: "Last 6 hours" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
];

interface LogViewerProps {
  logs: string[];
  filtered: string[];
  isLoading: boolean;
  isFetching: boolean;
  errorMessage: string | null;
  isCompact: boolean;
  logSearch: string;
  onLogSearchChange: (value: string) => void;
  logTimeRange: LogTimeRange;
  onLogTimeRangeChange: (value: LogTimeRange) => void;
  activeFilters: Set<"errors" | "warnings">;
  onToggleFilter: (f: "errors" | "warnings") => void;
  errCount: number;
  warnCount: number;
  onRefresh: () => void;
  onCopyLogs: () => void;
  copiedLogs: boolean;
}

const FILTER_CONFIGS = [
  { key: "errors" as const, label: "Errors", colorClass: "text-red-700" },
  { key: "warnings" as const, label: "Warnings", colorClass: "text-yellow-700" },
] as const;

export function LogViewer({
  logs,
  filtered,
  isLoading,
  isFetching,
  errorMessage,
  isCompact,
  logSearch,
  onLogSearchChange,
  logTimeRange,
  onLogTimeRangeChange,
  activeFilters,
  onToggleFilter,
  errCount,
  warnCount,
  onRefresh,
  onCopyLogs,
  copiedLogs,
}: LogViewerProps) {
  const counts = { errors: errCount, warnings: warnCount };

  return (
    <div>
      {/* Toolbar */}
      <div className={cn("flex items-center gap-1.5 px-3.5 py-2 bg-surface border-b border-border", isCompact ? "flex-wrap" : "flex-nowrap")}>
        {FILTER_CONFIGS.map((f) => {
          const active = activeFilters.has(f.key);
          return (
            <button
              key={f.key}
              onClick={() => onToggleFilter(f.key)}
              className={cn(
                "flex items-center gap-[5px] px-2 py-1 rounded-md border border-border cursor-pointer font-sans text-body-sm transition-all whitespace-nowrap",
                f.colorClass,
                active ? "bg-stone-200 font-medium" : "bg-transparent font-normal",
              )}
            >
              <span>{f.label}</span>
              <span className={cn("font-mono text-mono-sm", f.colorClass)}>
                {counts[f.key]}
              </span>
              {active && <X size={10} className="ml-0.5 shrink-0" />}
            </button>
          );
        })}
        <div className="flex-1" />
        <Select value={logTimeRange} onValueChange={(value) => onLogTimeRangeChange(value as LogTimeRange)}>
          <SelectTrigger className="h-8 w-auto min-w-[130px] px-3 font-sans text-body-sm bg-popover">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {LOG_TIME_RANGE_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="flex items-center gap-[5px] h-8 px-2.5 rounded-md border border-border bg-popover">
          <Search size={12} className="text-faint-foreground" />
          <input
            type="text"
            placeholder="Search logs"
            value={logSearch}
            onChange={(e) => onLogSearchChange(e.target.value)}
            className={cn(
              "bg-transparent border-none outline-none font-sans text-body-sm text-muted-foreground caret-teal-600",
              isCompact ? "w-[92px]" : "w-40",
            )}
          />
        </div>
        <button
          type="button"
          title="Refresh logs"
          onClick={onRefresh}
          className="flex items-center justify-center size-8 rounded border border-border bg-transparent text-foreground cursor-pointer"
        >
          <RefreshCw size={12} className={isFetching ? "dp-spin" : undefined} />
        </button>
        <button
          type="button"
          title="Copy logs"
          onClick={onCopyLogs}
          className="flex items-center justify-center size-8 rounded border border-border bg-transparent text-foreground cursor-pointer"
        >
          {copiedLogs ? <Check size={12} className="text-teal-600" /> : <Copy size={12} />}
        </button>
      </div>

      {/* Log output */}
      <div className="bg-stone-50 py-2.5 pb-3.5">
        {isLoading ? (
          <div className="flex items-center gap-2 px-[18px] py-3 font-mono text-mono-sm text-faint-foreground">
            <Loader2 size={14} className="dp-spin" />
            Loading logs…
          </div>
        ) : errorMessage ? (
          <div className="px-[18px] py-3 font-mono text-mono-sm text-coral-600 leading-relaxed">
            {errorMessage}
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-[18px] py-3 font-mono text-mono-sm text-faint-foreground">
            {logs.length === 0 ? "No log lines in this time window" : "No matching lines"}
          </div>
        ) : (
          filtered.map((line, li) => {
            const parsed = splitLogLineTimestamp(line);
            return (
              <div key={li} className="dp-log flex items-baseline py-px">
                <span className="font-mono text-mono-sm text-stone-500 min-w-[44px] text-right pr-3 shrink-0 select-none">
                  {li + 1}
                </span>
                <span className={cn("font-mono text-mono-sm text-faint-foreground pr-3 shrink-0", isCompact ? "min-w-32" : "min-w-[190px]")}>
                  {formatLogTimestamp(parsed.timestamp)}
                </span>
                <span className={cn("font-mono text-mono-md leading-[1.75] whitespace-pre-wrap break-words", logLineColorClass(line))}>
                  {parsed.message}
                </span>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
