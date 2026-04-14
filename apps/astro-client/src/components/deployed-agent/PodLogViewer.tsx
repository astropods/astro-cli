import { useState, useRef } from "react";
import { useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useDeploymentLogs } from "@/api/queries/deployments";
import { RefreshCw, Loader2, ArrowDown } from "lucide-react";
import type { WorkloadDetail, ApiError } from "@/lib/api";
import { useWindowVirtualizer } from "@tanstack/react-virtual";

const TIME_RANGES = [
  { value: '15m', label: 'Last 15 min' },
  { value: '1h',  label: 'Last 1 hour' },
  { value: '6h',  label: 'Last 6 hours' },
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d',  label: 'Last 7 days' },
];

export function PodLogViewer({ deploymentId, workload }: { deploymentId: string; workload: WorkloadDetail }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const containerParam = searchParams.get("container");
  const selectedContainer = containerParam && workload.containers.some((c) => c.name === containerParam)
    ? containerParam
    : workload.containers[0]?.name ?? "";
  const [timeRange, setTimeRange] = useState("1h");
  const { data: lines = [], isLoading, error: logsError, refetch } = useDeploymentLogs(
    deploymentId, workload.name, selectedContainer, timeRange,
  );
  const listRef = useRef<HTMLDivElement>(null);

  const virtualizer = useWindowVirtualizer({
    count: lines.length,
    estimateSize: () => 20,
    overscan: 60,
    scrollMargin: listRef.current?.offsetTop ?? 0,
  });

  const lineNumberWidth = String(lines.length).length;

  const scrollToBottom = () => {
    if (lines.length > 0) {
      virtualizer.scrollToIndex(lines.length - 1, { align: "end" });
    }
  };

  const error = logsError
    ? (logsError as unknown as ApiError & { details?: string }).details
      ?? (logsError as unknown as ApiError).error_description
      ?? logsError.message
      ?? "Failed to fetch logs"
    : null;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        {workload.containers.length > 1 && (
          <div className="flex items-center gap-1.5 text-sm">
            <span className="text-muted-foreground">Container:</span>
            <Select
              value={selectedContainer}
              onValueChange={(value) => setSearchParams((prev) => {
                const next = new URLSearchParams(prev);
                next.set("container", value);
                return next;
              })}
            >
              <SelectTrigger className="h-7 w-auto min-w-[120px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {workload.containers.map((c) => (
                  <SelectItem key={c.name} value={c.name}>{c.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <Select value={timeRange} onValueChange={setTimeRange}>
          <SelectTrigger className="h-7 w-auto min-w-[120px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIME_RANGES.map((r) => (
              <SelectItem key={r.value} value={r.value}>{r.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => refetch()} disabled={isLoading}>
            <RefreshCw size={14} className={isLoading ? "animate-spin" : ""} />
            Refresh
          </Button>
          <Button variant="outline" size="sm" onClick={scrollToBottom}>
            <ArrowDown size={14} />
            Bottom
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 size={24} className="animate-spin text-muted-foreground" />
          <span className="ml-2 text-muted-foreground">Loading logs...</span>
        </div>
      ) : error ? (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm rounded">
          {error}
        </div>
      ) : (
        <div ref={listRef} className="bg-stone-900 rounded">
          <div
            className="relative w-full"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => (
              <div
                key={virtualRow.index}
                data-index={virtualRow.index}
                ref={virtualizer.measureElement}
                className="absolute left-0 w-full flex text-xs font-mono leading-5 hover:bg-stone-800/60"
                style={{
                  top: virtualRow.start - (virtualizer.options.scrollMargin ?? 0),
                }}
              >
                <span
                  className="shrink-0 select-none text-right text-stone-500 px-3 border-r border-stone-700 py-px"
                  style={{ width: `${lineNumberWidth + 3}ch` }}
                >
                  {virtualRow.index + 1}
                </span>
                <span className="text-stone-100 px-3 whitespace-pre-wrap break-all min-w-0 py-px">
                  {lines[virtualRow.index]?.message ?? ""}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
