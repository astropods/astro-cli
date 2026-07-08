import { useMemo } from "react";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { cn } from "@/lib/utils";
import { summarize } from "@/lib/content-parse";
import { formatTimeAgo } from "@/lib/time-format";
import type { ReviewQueueItem, ReviewQueueSentiment } from "@/lib/api";

const SENTIMENT_DOT: Record<ReviewQueueSentiment, string> = {
  positive: "bg-success",
  negative: "bg-destructive",
  "": "bg-muted-foreground/60",
};

interface ReviewQueueListProps {
  items: ReviewQueueItem[];
  selectedId: string | null;
  onSelect: (traceId: string) => void;
  isLoading: boolean;
  isError: boolean;
  canLoadMore: boolean;
  isLoadingMore: boolean;
  onLoadMore: () => void;
}

export function ReviewQueueList({
  items,
  selectedId,
  onSelect,
  isLoading,
  isError,
  canLoadMore,
  isLoadingMore,
  onLoadMore,
}: ReviewQueueListProps) {
  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner delay={300} />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="px-6 py-12 text-center text-body-sm text-muted-foreground">
        Failed to load the queue.
      </div>
    );
  }

  if (items.length === 0) {
    if (canLoadMore) {
      return (
        <div className="flex flex-col items-center px-6 py-12 text-center">
          <div className="text-body-sm font-medium text-foreground">
            Ready for more traces
          </div>
          <p className="mt-1.5 text-body-sm text-muted-foreground">
            Load the next page of queue items to keep reviewing.
          </p>
          <div className="mt-4">
            <ReviewQueueLoadMoreButton
              isLoading={isLoadingMore}
              onLoadMore={onLoadMore}
            />
          </div>
        </div>
      );
    }

    return (
      <div className="px-6 py-12 text-center text-body-sm text-muted-foreground">
        No traces waiting for review.
      </div>
    );
  }

  return (
    <ReviewQueueListBody
      items={items}
      selectedId={selectedId}
      onSelect={onSelect}
      canLoadMore={canLoadMore}
      isLoadingMore={isLoadingMore}
      onLoadMore={onLoadMore}
    />
  );
}

function ReviewQueueListBody({
  items,
  selectedId,
  onSelect,
  canLoadMore,
  isLoadingMore,
  onLoadMore,
}: {
  items: ReviewQueueItem[];
  selectedId: string | null;
  onSelect: (traceId: string) => void;
  canLoadMore: boolean;
  isLoadingMore: boolean;
  onLoadMore: () => void;
}) {
  // Freeze relative timestamps to the moment items arrive; otherwise every
  // selection re-render bumps the "12s ago" → "13s ago" for recent rows.
  const agoByTraceId = useMemo(() => {
    const map = new Map<string, string>();
    for (const item of items) {
      map.set(item.trace_id, item.timestamp ? formatTimeAgo(item.timestamp) : "");
    }
    return map;
  }, [items]);

  return (
    <div className="flex min-h-full flex-col">
      <ul className="flex flex-col">
        {items.map((item) => (
          <li key={item.trace_id}>
            <ReviewQueueRow
              item={item}
              ago={agoByTraceId.get(item.trace_id) ?? ""}
              selected={selectedId === item.trace_id}
              onSelect={() => onSelect(item.trace_id)}
            />
          </li>
        ))}
      </ul>
      {canLoadMore && (
        <div className="mt-auto flex justify-center border-t border-border px-4 py-3">
          <ReviewQueueLoadMoreButton
            isLoading={isLoadingMore}
            onLoadMore={onLoadMore}
          />
        </div>
      )}
    </div>
  );
}

function ReviewQueueRow({
  item,
  ago,
  selected,
  onSelect,
}: {
  item: ReviewQueueItem;
  ago: string;
  selected: boolean;
  onSelect: () => void;
}) {
  const title = summarize(item.input);

  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        "flex min-h-16 w-full items-center gap-3 border-b border-l-2 border-border px-4 py-3.5 text-left transition-colors",
        selected
          ? "border-l-primary bg-primary/10"
          : "border-l-transparent hover:bg-muted/40",
      )}
    >
      <span
        aria-hidden
        className={cn(
          "size-2 flex-none rounded-full",
          SENTIMENT_DOT[item.sentiment] ?? SENTIMENT_DOT[""],
        )}
      />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="truncate text-body text-foreground" title={title}>
          {title}
        </div>
        {ago && (
          <div className="truncate font-mono text-mono-sm text-muted-foreground">{ago}</div>
        )}
      </div>
    </button>
  );
}

export function ReviewQueueLoadMoreButton({
  isLoading,
  onLoadMore,
}: {
  isLoading: boolean;
  onLoadMore: () => void;
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={isLoading}
      onClick={onLoadMore}
    >
      {isLoading ? "Loading more..." : "Load more items"}
    </Button>
  );
}
