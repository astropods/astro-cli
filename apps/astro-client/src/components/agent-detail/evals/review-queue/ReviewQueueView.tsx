import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { Check } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import { Spinner } from "@/components/ui/spinner";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import {
  useDatasetReviewQueue,
  usePostDatasetJudgment,
  useRemoveReviewQueueItem,
  useSetDatasetJudgmentCriteria,
  useUndoDatasetJudgment,
} from "@/api/queries/evals";
import type {
  DatasetJudgmentVerdict,
  EvalDatasetResponse,
  JudgmentCriterion,
  ReviewQueueItem,
  TraceEntry,
} from "@/lib/api";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "../EvalTabCard";
import { verdictHasCriteria } from "../judgment-criteria";
import { flyVerdictToGrade } from "../review-queue-motion";
import { JudgmentCriteriaPanel } from "./JudgmentCriteriaPanel";
import { QuickUndoToast } from "./QuickUndoToast";
import { ReviewQueueList } from "./ReviewQueueList";
import { ReviewQueueDetail, ReviewQueueDetailEmpty } from "./ReviewQueueDetail";
import { markedLabel } from "./ReviewQueueVerdictControls";
import {
  getAdjacentTraceIds,
  getBaselineStatus,
  getReviewQueuePageIndex,
  reviewQueueItemToTraceEntry,
  type BaselineStatus,
} from "./review-queue-utils";

const EMPTY_QUEUE_AUTO_LOAD_LIMIT = 3;
const EMPTY_REVIEW_QUEUE_ITEMS: ReviewQueueItem[] = [];

type ActiveJudgment = {
  traceId: string;
  verdict: DatasetJudgmentVerdict;
  item?: ReviewQueueItem;
  pageIndex?: number;
  nextTraceId?: string | null;
  nextReviewQueueItem?: ReviewQueueItem;
};

type ReviewQueuePanelAction = "none" | "open" | "sync";

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
  const [activeJudgment, setActiveJudgment] = useState<ActiveJudgment | null>(null);
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

  const activeVerdict =
    activeJudgment && selectedItem && activeJudgment.traceId === selectedItem.trace_id
      ? activeJudgment.verdict
      : null;

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

  const removeQueueItem = useRemoveReviewQueueItem(deploymentId);
  const commitJudgment = useCallback(
    (judgment: ActiveJudgment) => {
      removeQueueItem(judgment.traceId);
      if (selectedIdRef.current !== judgment.traceId) {
        return;
      }
      selectTraceId(judgment.nextTraceId ?? null);
      if (judgment.nextReviewQueueItem) {
        applyReviewQueueSelection(judgment.nextReviewQueueItem, "sync");
      } else {
        clearSyncedTracePanel();
      }
    },
    [
      applyReviewQueueSelection,
      clearSyncedTracePanel,
      removeQueueItem,
      selectTraceId,
    ],
  );

  const postJudgment = usePostDatasetJudgment(deploymentId, {
    onSuccess: (_data, variables) => {
      const judgment: ActiveJudgment = {
        traceId: variables.traceId,
        verdict: variables.verdict,
        item: variables.reviewQueueItem,
        pageIndex: variables.reviewQueuePageIndex,
        nextTraceId: variables.nextTraceId,
        nextReviewQueueItem: variables.nextReviewQueueItem,
      };
      setActiveJudgment(judgment);
      if (verdictHasCriteria(variables.verdict)) {
        return;
      }
      commitJudgment(judgment);
    },
  });

  const undoJudgment = useUndoDatasetJudgment(deploymentId);
  const setCriteria = useSetDatasetJudgmentCriteria(deploymentId);
  const resolvedAgentAvatarUrl =
    avatarBust ?? agentAvatarUrl ?? getDeploymentAvatarUrl(deploymentId);

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
    if (
      activeJudgment &&
      verdictHasCriteria(activeJudgment.verdict) &&
      activeJudgment.traceId !== traceId
    ) {
      removeQueueItem(activeJudgment.traceId);
    }
    postJudgment.reset();
    undoJudgment.reset();
    setCriteria.reset();
    setActiveJudgment(null);
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
    setCriteria.reset();
    setActiveJudgment(null);
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

  const handleUndo = () => {
    if (!activeJudgment) {
      return;
    }

    const { traceId } = activeJudgment;
    undoJudgment.reset();
    undoJudgment.mutate(
      {
        traceId,
        reviewQueueItem: activeJudgment.item,
        reviewQueuePageIndex: activeJudgment.pageIndex,
      },
      {
        onSuccess: () => {
          setActiveJudgment(null);
          if (activeJudgment.item) {
            applyReviewQueueSelection(activeJudgment.item, "sync");
          } else {
            selectTraceId(traceId);
          }
        },
      },
    );
  };

  const handleCriteriaDone = (criteria: JudgmentCriterion[]) => {
    if (!activeJudgment) {
      return;
    }

    const judgment = activeJudgment;
    const finish = () => {
      commitJudgment(judgment);
      setActiveJudgment(null);
    };

    if (criteria.length === 0) {
      finish();
      return;
    }
    setCriteria.mutate(
      { traceId: judgment.traceId, criteria },
      { onSuccess: finish },
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
                activeVerdict={activeVerdict}
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
      {activeJudgment &&
        (verdictHasCriteria(activeJudgment.verdict) ? (
          <JudgmentCriteriaPanel
            key={activeJudgment.traceId}
            verdict={activeJudgment.verdict}
            title={markedLabel(activeJudgment.verdict)}
            isUndoing={undoJudgment.isPending}
            isSaving={setCriteria.isPending}
            isError={setCriteria.isError}
            onUndo={handleUndo}
            onSave={handleCriteriaDone}
          />
        ) : (
          <QuickUndoToast
            key={activeJudgment.traceId}
            label={markedLabel(activeJudgment.verdict)}
            isUndoing={undoJudgment.isPending}
            onUndo={handleUndo}
            onDismiss={() => setActiveJudgment(null)}
          />
        ))}
    </>
  );
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
