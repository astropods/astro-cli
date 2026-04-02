import { useMemo, useState } from "react";
import { Loader2, X } from "lucide-react";
import { MagnifyingGlassIcon, ArrowPathIcon } from "@heroicons/react/24/outline";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { logLineColorClass, splitLogLineTimestamp, formatLogTimestamp } from "@/lib/log-utils";
import { useLogFiltering } from "@/hooks/use-log-filtering";
import { useDeploymentLogs } from "@/api/queries/deployments";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import type { ApiError } from "@/lib/api";
import type { DeployHistoryStatus } from "./history/types";

export type LogTimeRange = "15m" | "1h" | "6h" | "24h" | "7d";

const LOG_TIME_RANGE_OPTIONS: { value: LogTimeRange; label: string }[] = [
  { value: "15m", label: "Last 15 min" },
  { value: "1h", label: "Last 1 hour" },
  { value: "6h", label: "Last 6 hours" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
];

interface LogViewerProps {
  deploymentId: string;
  workloadName: string;
  selectedContainer: string;
  deploymentStatus: DeployHistoryStatus;
  isOpen: boolean;
  isCompact: boolean;
  onRestart?: () => void;
  isRestarting?: boolean;
}

const FILTER_CONFIGS = [
  { key: "errors" as const, label: "Errors", colorClass: "text-[var(--color-coral-600)]" },
  { key: "warnings" as const, label: "Warnings", colorClass: "text-yellow-500" },
] as const;

export function LogViewer({
  deploymentId,
  workloadName,
  selectedContainer,
  deploymentStatus,
  isOpen,
  isCompact,
  onRestart,
  isRestarting = false,
}: LogViewerProps) {
  const [logSearch, setLogSearch] = useState("");
  const [logTimeRange, setLogTimeRange] = useState<LogTimeRange>("24h");

  const { data: logsRaw, isLoading, error } = useDeploymentLogs(
    deploymentId,
    workloadName,
    selectedContainer,
    logTimeRange,
    { enabled: isOpen && !!selectedContainer, refetchInterval: isOpen && (deploymentStatus === "deploying" || deploymentStatus === "restarting") ? 3000 : false },
  );

  const logs = useMemo(() => {
    const raw = logsRaw ?? "";
    return raw ? raw.split("\n") : [];
  }, [logsRaw]);

  const errorMessage = error
    ? (error as unknown as ApiError & { details?: string }).details ??
      (error as unknown as ApiError).error_description ??
      (error as Error).message ??
      "Failed to fetch logs"
    : null;

  const { activeFilters, toggleFilter, errCount, warnCount, filtered } = useLogFiltering(logs, logSearch);

  const counts = { errors: errCount, warnings: warnCount };

  return (
    <div>
      <div className={cn("flex items-center gap-1.5 px-3.5 py-2 bg-surface border-b border-border", isCompact ? "flex-wrap" : "flex-nowrap")}>
        {FILTER_CONFIGS.map((f) => {
          const active = activeFilters.has(f.key);
          return (
            <button
              key={f.key}
              onClick={() => toggleFilter(f.key)}
              className={cn(
                "flex items-center gap-[5px] px-2 py-1 rounded-[calc(var(--radius-sm)+2px)] border border-border cursor-pointer font-sans text-body-sm transition-all whitespace-nowrap",
                f.colorClass,
                active ? "bg-muted font-medium" : "bg-transparent font-normal",
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
        <Select value={logTimeRange} onValueChange={(value) => setLogTimeRange(value as LogTimeRange)}>
          <SelectTrigger className="h-8 w-auto min-w-[130px] px-3 font-sans text-body-sm bg-popover rounded-[calc(var(--radius-sm)+2px)]">
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
        {onRestart && (
          <Button
            variant="outline"
            size="sm"
            title="Restart this pod"
            disabled={isRestarting}
            onClick={onRestart}
          >
            <ArrowPathIcon className={cn("size-3.5", isRestarting && "dp-spin")} />
            {isRestarting ? "Restarting…" : "Restart"}
          </Button>
        )}
        <CopyButton copyText={() => logs.join("\n")} title="Copy logs" resetMs={900} />
      </div>

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
