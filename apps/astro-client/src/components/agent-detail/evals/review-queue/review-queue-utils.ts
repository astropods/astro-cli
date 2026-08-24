import type {
  ReviewQueueItem,
  ReviewQueueResponse,
  TraceEntry,
} from "@/lib/api";

export function reviewQueueItemToTraceEntry(item: ReviewQueueItem): TraceEntry {
  return {
    trace_id: item.trace_id,
    name: "Review queue trace",
    status: "success",
    latency_ms: 0,
    total_cost: 0,
    timestamp: item.timestamp,
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
