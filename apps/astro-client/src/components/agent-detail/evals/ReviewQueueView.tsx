import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type ReactNode,
  type RefObject,
} from "react";
import type { LucideIcon } from "lucide-react";
import {
  ArrowRight,
  Check,
  Frown,
  Meh,
  Minus,
  Smile,
  UserRound,
  X,
} from "lucide-react";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
import { ContentSection } from "@/components/agent-detail/traces/detail/ContentSection";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { summarize } from "@/lib/content-parse";
import { formatTimeAgo } from "@/lib/time-format";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import {
  useDatasetReviewQueue,
  usePostDatasetJudgment,
  useUndoDatasetJudgment,
} from "@/api/queries/evals";
import type {
  DatasetJudgmentVerdict,
  EvalDatasetResponse,
  ReviewQueueItem,
  ReviewQueueResponse,
  ReviewQueueSentiment,
  TraceEntry,
} from "@/lib/api";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "./EvalTabCard";
import { flyVerdictToGrade } from "./review-queue-motion";

const REVIEW_QUEUE_VERDICT_OPTIONS: Array<{
  verdict: DatasetJudgmentVerdict;
  label: string;
  shortcut: string;
  Icon: LucideIcon;
  iconClassName: string;
}> = [
  {
    verdict: "good",
    label: "Good",
    shortcut: "G",
    Icon: Check,
    iconClassName: "text-success",
  },
  {
    verdict: "bad",
    label: "Bad",
    shortcut: "B",
    Icon: X,
    iconClassName: "text-destructive",
  },
  {
    verdict: "unknown",
    label: "Neutral",
    shortcut: "N",
    Icon: Minus,
    iconClassName: "text-muted-foreground",
  },
];

const REVIEW_QUEUE_VERDICT_SHORTCUTS: Record<string, DatasetJudgmentVerdict> = {
  g: "good",
  b: "bad",
  n: "unknown",
};

const EMPTY_QUEUE_AUTO_LOAD_LIMIT = 3;
const EMPTY_REVIEW_QUEUE_ITEMS: ReviewQueueItem[] = [];
// Let the dataset page own vertical scrolling; resized sections still scroll
// once the user gives them an explicit height.
const REVIEW_QUEUE_CONTENT_CLASS =
  "dp-scroll overflow-y-auto overscroll-contain";

type QuickUndoJudgment = {
  traceId: string;
  verdict: DatasetJudgmentVerdict;
  item?: ReviewQueueItem;
  pageIndex?: number;
};

type ReviewQueuePanelAction = "none" | "open" | "sync";

type BaselineStatus = {
  label: string;
  tooltip: string;
  color: StatusBadgeColor;
};

export interface ReviewQueueViewProps {
  deploymentId: string;
  account: string;
  agentName: string;
  agentLabel: string;
  agentAvatarUrl?: string;
  summary: EvalDatasetResponse;
  gradeTargetRef?: RefObject<HTMLDivElement | null>;
  onOpenTrace?: (trace: TraceEntry) => void;
  onSelectedTraceChange?: (trace: TraceEntry) => void;
  onSelectedTraceCleared?: () => void;
}

export function ReviewQueueView({
  deploymentId,
  account,
  agentName,
  agentLabel,
  agentAvatarUrl,
  summary,
  gradeTargetRef,
  onOpenTrace,
  onSelectedTraceChange,
  onSelectedTraceCleared,
}: ReviewQueueViewProps) {
  const {
    data,
    isLoading,
    isError,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useDatasetReviewQueue(deploymentId);
  const avatarBust = useDeploymentAvatarBust(deploymentId);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [quickUndo, setQuickUndo] = useState<QuickUndoJudgment | null>(null);
  // Mirrors selectedId for synchronous reads inside mutation callbacks.
  const selectedIdRef = useRef<string | null>(null);
  // Tracks the trace currently shown in the open detail panel.
  const syncedPanelTraceIdRef = useRef<string | null>(null);
  const emptyQueueAutoLoadCountRef = useRef(0);
  const items = useMemo(
    () => data?.pages.flatMap((page) => page.items) ?? EMPTY_REVIEW_QUEUE_ITEMS,
    [data?.pages],
  );
  const loadedPageCount = data?.pages.length ?? 0;
  const selectedItem =
    items.find((item) => item.trace_id === selectedId) ?? items[0] ?? null;
  const selectedIndex = selectedItem
    ? items.findIndex((item) => item.trace_id === selectedItem.trace_id)
    : -1;
  const baselineStatus = getBaselineStatus(summary);
  const canLoadMore = Boolean(hasNextPage);
  const selectTraceId = useCallback((traceId: string | null) => {
    selectedIdRef.current = traceId;
    setSelectedId(traceId);
  }, []);
  // Single adapter for queue selection plus optional panel open/sync work.
  const applyReviewQueueSelection = useCallback(
    (item: ReviewQueueItem, panelAction: ReviewQueuePanelAction = "none") => {
      selectTraceId(item.trace_id);
      if (panelAction === "none") {
        return;
      }

      let panelCallback: ((trace: TraceEntry) => void) | undefined;
      if (panelAction === "open") {
        panelCallback = onOpenTrace;
      }
      if (panelAction === "sync") {
        panelCallback = onSelectedTraceChange;
      }

      // Without a panel callback, sync remains row-only and never opens the panel.
      if (!panelCallback) {
        return;
      }

      syncedPanelTraceIdRef.current = item.trace_id;
      panelCallback(reviewQueueItemToTraceEntry(item));
    },
    [onOpenTrace, onSelectedTraceChange, selectTraceId],
  );
  const clearSyncedTracePanel = useCallback((clearSelection = false) => {
    if (clearSelection) {
      selectTraceId(null);
    }
    syncedPanelTraceIdRef.current = null;
    onSelectedTraceCleared?.();
  }, [onSelectedTraceCleared, selectTraceId]);
  const postJudgment = usePostDatasetJudgment(deploymentId, {
    onSuccess: (_data, variables) => {
      setQuickUndo({
        traceId: variables.traceId,
        verdict: variables.verdict,
        item: variables.reviewQueueItem,
        pageIndex: variables.reviewQueuePageIndex,
      });
      if (selectedIdRef.current !== variables.traceId) {
        return;
      }

      selectTraceId(variables.nextTraceId ?? null);
      if (variables.nextReviewQueueItem) {
        applyReviewQueueSelection(variables.nextReviewQueueItem, "sync");
      } else {
        clearSyncedTracePanel();
      }
    },
  });
  const undoJudgment = useUndoDatasetJudgment(deploymentId);
  const resolvedAgentAvatarUrl =
    avatarBust ?? agentAvatarUrl ?? getDeploymentAvatarUrl(deploymentId);

  useEffect(() => {
    if (!quickUndo) {
      return;
    }

    const timer = window.setTimeout(() => setQuickUndo(null), 8000);
    return () => window.clearTimeout(timer);
  }, [quickUndo]);

  useEffect(() => {
    if (items.length > 0 || !hasNextPage) {
      emptyQueueAutoLoadCountRef.current = 0;
      return;
    }

    if (
      isLoading ||
      isError ||
      isFetchingNextPage ||
      emptyQueueAutoLoadCountRef.current >= EMPTY_QUEUE_AUTO_LOAD_LIMIT
    ) {
      return;
    }

    emptyQueueAutoLoadCountRef.current += 1;
    void fetchNextPage();
  }, [
    fetchNextPage,
    hasNextPage,
    isError,
    isFetchingNextPage,
    isLoading,
    items.length,
    loadedPageCount,
  ]);

  // While the panel is open, reconcile cache changes only when the selected
  // trace is removed or replaced; reorders keep the explicit selection pinned.
  useEffect(() => {
    if (!onSelectedTraceChange) {
      return;
    }

    if (!selectedItem) {
      if (syncedPanelTraceIdRef.current !== null) {
        clearSyncedTracePanel(true);
      }
      return;
    }

    if (syncedPanelTraceIdRef.current === selectedItem.trace_id) {
      return;
    }

    applyReviewQueueSelection(selectedItem, "sync");
  }, [
    applyReviewQueueSelection,
    clearSyncedTracePanel,
    onSelectedTraceChange,
    selectedItem,
  ]);

  const handleSelectTrace = (traceId: string) => {
    postJudgment.reset();
    undoJudgment.reset();
    setQuickUndo(null);
    const item = items.find((candidate) => candidate.trace_id === traceId);
    if (item) {
      applyReviewQueueSelection(item, "sync");
    } else {
      selectTraceId(traceId);
    }
  };
  const handleJudgeTrace = (
    traceId: string,
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLButtonElement | null,
  ) => {
    const { previousTraceId, nextTraceId } = getAdjacentTraceIds(items, traceId);
    const nextSelectedTraceId = nextTraceId ?? previousTraceId;
    const reviewQueueItem = items.find((item) => item.trace_id === traceId);
    const nextReviewQueueItem = nextSelectedTraceId
      ? items.find((item) => item.trace_id === nextSelectedTraceId)
      : undefined;
    const reviewQueuePageIndex = getReviewQueuePageIndex(data?.pages, traceId);
    setQuickUndo(null);
    flyVerdictToGrade(
      trigger?.getBoundingClientRect() ?? null,
      gradeTargetRef?.current,
      verdict,
    );

    if (selectedItem?.trace_id === traceId) {
      selectTraceId(selectedIdRef.current ?? traceId);
    }
    postJudgment.mutate({
      traceId,
      verdict,
      nextTraceId: nextSelectedTraceId,
      reviewQueueItem,
      nextReviewQueueItem,
      reviewQueuePageIndex,
    });
  };
  const handleQuickUndo = () => {
    if (!quickUndo) {
      return;
    }

    const { traceId } = quickUndo;
    undoJudgment.reset();
    undoJudgment.mutate(
      {
        traceId,
        reviewQueueItem: quickUndo.item,
        reviewQueuePageIndex: quickUndo.pageIndex,
      },
      {
        onSuccess: () => {
          setQuickUndo(null);
          if (quickUndo.item) {
            applyReviewQueueSelection(quickUndo.item, "sync");
          } else {
            selectTraceId(traceId);
          }
        },
      },
    );
  };
  const handleLoadMore = () => {
    void fetchNextPage();
  };

  return (
    <>
      <EvalTabCard className="@container/review-card min-h-0">
        <EvalTabCardHeader label="Review queue" datasetName={summary.dataset_name}>
          {baselineStatus && <BaselineStatusBadge status={baselineStatus} />}
        </EvalTabCardHeader>
        <EvalTabCardBody className="flex-col @[760px]/review-card:flex-row">
          <aside className="dp-scroll flex max-h-64 w-full flex-none flex-col overflow-y-auto border-b border-border bg-card dark:bg-surface @[760px]/review-card:max-h-none @[760px]/review-card:w-[clamp(18rem,34%,24.5rem)] @[760px]/review-card:border-b-0 @[760px]/review-card:border-r">
            <ReviewQueueList
              items={items}
              selectedId={selectedItem?.trace_id ?? null}
              onSelect={handleSelectTrace}
              isLoading={isLoading}
              isError={isError}
              canLoadMore={canLoadMore}
              isLoadingMore={isFetchingNextPage}
              onLoadMore={handleLoadMore}
            />
          </aside>

          <div className="flex min-w-0 flex-1 flex-col bg-background">
            {isLoading ? (
              <div className="flex flex-1 items-center justify-center">
                <Spinner delay={300} />
              </div>
            ) : isError ? (
              <div className="flex flex-1 flex-col items-center justify-center p-12 text-center">
                <div className="text-heading-3 font-semibold text-foreground">
                  Queue unavailable
                </div>
                <p className="mt-1.5 text-body-sm text-muted-foreground">
                  Failed to load the trace review queue.
                </p>
              </div>
            ) : selectedItem ? (
              <ReviewQueueDetail
                item={selectedItem}
                account={account}
                agentName={agentName}
                agentLabel={agentLabel}
                agentAvatarUrl={resolvedAgentAvatarUrl}
                onJudge={handleJudgeTrace}
                isJudging={postJudgment.isPending}
                showJudgmentError={postJudgment.isError || undoJudgment.isError}
                position={selectedIndex >= 0 ? selectedIndex + 1 : 0}
                queueSize={items.length}
                onOpenTrace={
                  onOpenTrace
                    ? () => applyReviewQueueSelection(selectedItem, "open")
                    : undefined
                }
              />
            ) : (
              <ReviewQueueDetailEmpty
                showJudgmentError={postJudgment.isError || undoJudgment.isError}
                canLoadMore={canLoadMore}
                isLoadingMore={isFetchingNextPage}
                onLoadMore={handleLoadMore}
              />
            )}
          </div>
        </EvalTabCardBody>
      </EvalTabCard>
      <ReviewQueueQuickUndoToast
        quickUndo={quickUndo}
        isUndoing={undoJudgment.isPending}
        onQuickUndo={handleQuickUndo}
      />
    </>
  );
}

function getBaselineStatus(summary: EvalDatasetResponse): BaselineStatus | null {
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

function BaselineStatusBadge({ status }: { status: BaselineStatus }) {
  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className="inline-flex rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            tabIndex={0}
          >
            <StatusBadge
              color={status.color}
              outline
              className="gap-1.5 px-2.5 py-1 font-sans text-body-sm font-medium normal-case tracking-normal"
            >
              <Check aria-hidden className="size-3.5" />
              {status.label}
            </StatusBadge>
          </span>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-64 text-left">
          {status.tooltip}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function reviewQueueItemToTraceEntry(item: ReviewQueueItem): TraceEntry {
  return {
    trace_id: item.trace_id,
    name: "Review queue trace",
    status: "success",
    latency_ms: 0,
    total_cost: 0,
    input: traceEntryContent(item.input),
    output: traceEntryContent(item.output),
    timestamp: item.timestamp,
  };
}

function traceEntryContent(content: unknown) {
  if (typeof content === "string") {
    return content;
  }
  if (content == null) {
    return "";
  }

  try {
    return JSON.stringify(content, null, 2);
  } catch {
    return String(content);
  }
}

function getAdjacentTraceIds(
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

function getReviewQueuePageIndex(
  pages: ReviewQueueResponse[] | undefined,
  traceId: string,
) {
  const index = pages?.findIndex((page) =>
    page.items.some((item) => item.trace_id === traceId),
  );
  return index != null && index >= 0 ? index : undefined;
}

function getReviewQueueShortcutVerdict(event: KeyboardEvent) {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    isEditableShortcutTarget(event.target)
  ) {
    return null;
  }

  return REVIEW_QUEUE_VERDICT_SHORTCUTS[event.key.toLowerCase()] ?? null;
}

function isEditableShortcutTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  return (
    target.isContentEditable ||
    Boolean(
      target.closest(
        "input, textarea, select, [contenteditable='true'], [contenteditable='plaintext-only']",
      ),
    )
  );
}

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

function ReviewQueueList({
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

const SENTIMENT_DOT: Record<ReviewQueueSentiment, string> = {
  positive: "bg-success",
  negative: "bg-destructive",
  "": "bg-muted-foreground/60",
};

const SENTIMENT_BADGE: Record<
  ReviewQueueSentiment,
  { label: string; color: "success" | "error" | "muted"; Icon: LucideIcon }
> = {
  positive: { label: "Likely positive", color: "success", Icon: Smile },
  negative: { label: "Likely negative", color: "error", Icon: Frown },
  "": { label: "No signal", color: "muted", Icon: Meh },
};

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

function ReviewQueueDetail({
  item,
  account,
  agentName,
  agentLabel,
  agentAvatarUrl,
  onJudge,
  isJudging,
  showJudgmentError,
  position,
  queueSize,
  onOpenTrace,
}: {
  item: ReviewQueueItem;
  account: string;
  agentName: string;
  agentLabel: string;
  agentAvatarUrl: string;
  onJudge: (
    traceId: string,
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLButtonElement | null,
  ) => void;
  isJudging: boolean;
  showJudgmentError: boolean;
  position: number;
  queueSize: number;
  onOpenTrace?: () => void;
}) {
  const sentiment = SENTIMENT_BADGE[item.sentiment] ?? SENTIMENT_BADGE[""];
  const traceLabel = truncateTraceId(item.trace_id);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-none flex-col items-start justify-between gap-3 border-b border-border px-4 py-4 @[520px]/review-card:px-6 @[640px]/review-card:flex-row @[640px]/review-card:items-center @[640px]/review-card:gap-4">
        <div className="flex min-w-0 items-center">
          <div className="flex min-w-0 flex-wrap items-center gap-3">
            <TraceDetailHoverLink
              traceId={item.trace_id}
              traceLabel={traceLabel}
              onOpenTrace={onOpenTrace}
            />
            <TooltipProvider delayDuration={300}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="inline-flex cursor-help">
                    <StatusBadge
                      color={sentiment.color}
                      className="flex-none gap-1.5 px-2.5 py-1 font-sans text-label font-medium tracking-normal"
                    >
                      <sentiment.Icon aria-hidden className="size-3" />
                      <span className="underline decoration-current/45 decoration-dotted underline-offset-2">
                        {sentiment.label}
                      </span>
                    </StatusBadge>
                  </span>
                </TooltipTrigger>
                <TooltipContent className="max-w-56 text-left">
                  Inferred from keywords in the user's next reply. Only affects queue order,
                  never the verdict.
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
        </div>
        <div className="flex flex-none items-center gap-2">
          <ReviewQueueDetailNavigation
            position={position}
            total={queueSize}
          />
        </div>
      </div>

      <div
        data-review-queue-detail
        className="min-h-0 flex-1 p-4 @[520px]/review-card:p-6"
      >
        <div className="mx-auto flex max-w-3xl flex-col gap-5">
          <ContentSection
            label="User"
            content={item.input}
            icon={<UserSectionIcon />}
            mode="pretty"
            contentClassName={REVIEW_QUEUE_CONTENT_CLASS}
            resizableContent
          />
          <ContentSection
            label={agentLabel}
            content={item.output}
            mode="pretty"
            contentClassName={REVIEW_QUEUE_CONTENT_CLASS}
            resizableContent
            icon={
              <BlueprintIdentity
                account={account}
                name={agentName}
                size={20}
                url={agentAvatarUrl}
                className="size-5 rounded-sm"
              />
            }
          />
        </div>
      </div>

      <ReviewQueueVerdictFooter
        isPending={isJudging}
        showError={showJudgmentError}
        onSelect={(verdict, trigger) => onJudge(item.trace_id, verdict, trigger)}
      />
    </div>
  );
}

function TraceDetailHoverLink({
  traceId,
  traceLabel,
  onOpenTrace,
}: {
  traceId: string;
  traceLabel: string;
  onOpenTrace?: () => void;
}) {
  if (!onOpenTrace) {
    return (
      <span
        className="font-mono text-mono-sm text-muted-foreground"
        title={traceId}
      >
        {traceLabel}
      </span>
    );
  }

  return (
    <button
      type="button"
      aria-label={`View ${traceLabel}`}
      title={traceId}
      onClick={onOpenTrace}
      className={cn(
        "group/trace -ml-1.5 inline-flex cursor-pointer items-center gap-1.5 rounded-sm px-1.5 py-1 text-foreground transition-colors",
        "hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
      )}
    >
      <span className="font-sans text-body-sm font-medium">
        View {traceLabel}
      </span>
      <ArrowRight
        aria-hidden
        className="size-3.5 transition-transform group-hover/trace:translate-x-0.5 group-focus-visible/trace:translate-x-0.5"
      />
    </button>
  );
}

function ReviewQueueDetailNavigation({
  position,
  total,
}: {
  position: number;
  total: number;
}) {
  if (total <= 0) {
    return null;
  }

  return (
    <span
      className="min-w-12 text-center font-mono text-mono-sm text-muted-foreground tabular-nums"
      aria-label={`Trace ${position} of ${total}`}
    >
      {position} / {total}
    </span>
  );
}

function ReviewQueueVerdictFooter({
  isPending,
  showError,
  onSelect,
}: {
  isPending: boolean;
  showError: boolean;
  onSelect: (
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLButtonElement | null,
  ) => void;
}) {
  const verdictButtonRefs = useRef<
    Record<DatasetJudgmentVerdict, HTMLButtonElement | null>
  >({
    good: null,
    bad: null,
    unknown: null,
  });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const verdict = getReviewQueueShortcutVerdict(event);
      if (!verdict || isPending) {
        return;
      }

      event.preventDefault();
      onSelect(verdict, verdictButtonRefs.current[verdict]);
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isPending, onSelect]);

  return (
    <div className="flex flex-none flex-wrap items-center justify-between gap-3 border-t border-border px-4 py-4 @[520px]/review-card:px-6 @max-[520px]/review-card:flex-col @max-[520px]/review-card:items-stretch">
      <div className="min-w-0 flex-1 text-body-sm text-muted-foreground @max-[520px]/review-card:flex-none">
        {showError ? (
          "Could not save verdict. Try again."
        ) : (
          <>
            Select an option or use{" "}
            <ShortcutKey className="mx-0.5">G</ShortcutKey>
            {" / "}
            <ShortcutKey className="mx-0.5">B</ShortcutKey>
            {" / "}
            <ShortcutKey className="mx-0.5">N</ShortcutKey>.
          </>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2 @max-[520px]/review-card:grid @max-[520px]/review-card:grid-cols-1">
        {REVIEW_QUEUE_VERDICT_OPTIONS.map(
          ({ verdict, label, shortcut, Icon, iconClassName }) => (
            <Button
              key={verdict}
              ref={(node) => {
                verdictButtonRefs.current[verdict] = node;
              }}
              type="button"
              variant="outline"
              size="sm"
              disabled={isPending}
              onClick={(event: MouseEvent<HTMLButtonElement>) =>
                onSelect(verdict, event.currentTarget)
              }
              className="@max-[520px]/review-card:w-full"
            >
              <Icon className={cn("size-4", iconClassName)} />
              {label}
              <ShortcutKey ariaHidden>{shortcut}</ShortcutKey>
            </Button>
          ),
        )}
      </div>
    </div>
  );
}

function quickUndoLabel(verdict: DatasetJudgmentVerdict) {
  const label =
    REVIEW_QUEUE_VERDICT_OPTIONS.find((option) => option.verdict === verdict)
      ?.label ?? "verdict";
  return `Marked as ${label.toLowerCase()}`;
}

function ReviewQueueQuickUndoToast({
  quickUndo,
  isUndoing,
  onQuickUndo,
}: {
  quickUndo: QuickUndoJudgment | null;
  isUndoing: boolean;
  onQuickUndo: () => void;
}) {
  if (!quickUndo) {
    return null;
  }

  return (
    <div
      aria-live="polite"
      className="fixed bottom-6 left-1/2 z-50 flex w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 animate-in items-center justify-between gap-3 rounded-md border border-border bg-card py-2.5 pl-4 pr-3 text-body-sm text-foreground shadow-xl fade-in slide-in-from-bottom-2 duration-200 dark:bg-surface sm:w-auto sm:min-w-[300px]"
    >
      <span className="min-w-0 truncate">{quickUndoLabel(quickUndo.verdict)}</span>
      <Button
        type="button"
        variant="outline"
        size="xs"
        disabled={isUndoing}
        onClick={onQuickUndo}
        className="h-7 flex-none px-3 font-semibold"
      >
        {isUndoing ? "Undoing..." : "Undo"}
      </Button>
    </div>
  );
}

function ShortcutKey({
  children,
  className,
  ariaHidden = false,
}: {
  children: ReactNode;
  className?: string;
  ariaHidden?: boolean;
}) {
  return (
    <kbd
      aria-hidden={ariaHidden || undefined}
      className={cn(
        "inline-flex h-5 min-w-5 items-center justify-center rounded border border-border-strong bg-muted/40 px-1.5 font-mono text-[11px] font-medium leading-none text-muted-foreground",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

function truncateTraceId(traceId: string) {
  return `trace_${traceId.replace(/^trace_/, "").slice(0, 6)}`;
}

function UserSectionIcon() {
  return (
    <span
      aria-hidden
      className="flex size-5 items-center justify-center rounded-full border border-border bg-muted text-muted-foreground"
    >
      <UserRound className="size-3" />
    </span>
  );
}

function ReviewQueueDetailEmpty({
  showJudgmentError,
  canLoadMore,
  isLoadingMore,
  onLoadMore,
}: {
  showJudgmentError: boolean;
  canLoadMore: boolean;
  isLoadingMore: boolean;
  onLoadMore: () => void;
}) {
  const title = canLoadMore ? "Ready for more traces" : "You're all caught up";
  const description = canLoadMore
    ? "Load the next page of queue items to keep reviewing."
    : "Every trace has a verdict.";

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-1 flex-col items-center justify-center gap-3.5 p-6 text-center @[520px]/review-card:p-12">
        <span
          aria-hidden
          className="flex size-12 items-center justify-center rounded-full bg-success/10 text-success"
        >
          <Check className="size-6" />
        </span>
        <div>
          <div className="text-heading-3 font-semibold text-foreground">{title}</div>
          <p className="mt-1.5 text-body-sm text-muted-foreground">{description}</p>
        </div>
        {canLoadMore && (
          <ReviewQueueLoadMoreButton
            isLoading={isLoadingMore}
            onLoadMore={onLoadMore}
          />
        )}
      </div>
      {showJudgmentError && (
        <div className="flex flex-none items-center border-t border-border px-6 py-4 text-body-sm text-muted-foreground">
          Could not save verdict. Try again.
        </div>
      )}
    </div>
  );
}

function ReviewQueueLoadMoreButton({
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
