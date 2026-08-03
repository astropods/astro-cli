import type { StatusBadgeColor } from "@/components/StatusBadge";
import type {
  EvalDatasetResponse,
  ReviewQueueItem,
  ReviewQueueResponse,
  TraceEntry,
} from "@/lib/api";

export type BaselineStatus = {
  label: string;
  tooltip: string;
  color: StatusBadgeColor;
};

export function getBaselineStatus(
  summary: EvalDatasetResponse,
): BaselineStatus | null {
  switch (summary.grade.toUpperCase()) {
    case "A":
      return {
        label: "Strong coverage",
        tooltip:
          "You've labeled a representative sample. Keep going to capture edge cases and strengthen future evals.",
        color: "success",
      };
    case "B":
      return {
        label: "Good coverage",
        tooltip:
          "You've labeled a solid sample of traces. Keep going to capture edge cases and push toward an A.",
        color: "success",
      };
    case "C":
      return {
        label: "Enough coverage",
        tooltip:
          "You've labeled enough traces to get started. Keep going to improve coverage and reliability.",
        color: "success",
      };
    default:
      return null;
  }
}

export function reviewQueueItemToTraceEntry(item: ReviewQueueItem): TraceEntry {
  return {
    trace_id: item.trace_id,
    name: "Review queue trace",
    status: "success",
    latency_ms: 0,
    total_cost: 0,
    timestamp: item.timestamp,
    user_id: item.user_id,
    user_details: item.user_details,
  };
}

export function getAdjacentTraceIds(
  items: ReviewQueueItem[],
  traceId: string | null,
) {
  const index = traceId
    ? items.findIndex((item) => item.trace_id === traceId)
    : -1;

  return {
    previousTraceId:
      index > 0 ? items[index - 1]?.trace_id ?? null : null,
    nextTraceId:
      index >= 0 && index < items.length - 1
        ? items[index + 1]?.trace_id ?? null
        : null,
  };
}

export function getReviewQueuePageIndex(
  pages: ReviewQueueResponse[] | undefined,
  traceId: string,
) {
  const index = pages?.findIndex((page) =>
    page.items.some((item) => item.trace_id === traceId),
  );
  return index != null && index >= 0 ? index : undefined;
}

export function truncateTraceId(traceId: string) {
  return `trace_${traceId.replace(/^trace_/, "").slice(0, 6)}`;
}
