import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { Check } from "lucide-react";
import { toast } from "sonner";
import { StatusBadge } from "@/components/StatusBadge";
import { Spinner } from "@/components/ui/spinner";
import { WarningPanel } from "@/components/ui/status-panel";
import { cn } from "@/lib/utils";
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
  useReviewQueuePredictionStatus,
  useSetDatasetJudgmentCriteria,
  useUndoDatasetJudgment,
} from "@/api/queries/evals";
import type {
  DatasetJudgmentVerdict,
  EvalDatasetResponse,
  JudgmentCriterion,
  ReviewQueueItem,
  ReviewQueuePredictionFilter,
  TraceEntry,
} from "@/lib/api";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "../EvalTabCard";
import {
  predictedCriterionKeysForVerdict,
  verdictHasCriteria,
} from "../judgment-criteria";
import { flyVerdictToGrade } from "../review-queue-motion";
import { JudgmentCriteriaPanel } from "./JudgmentCriteriaPanel";
import { QuickUndoToast } from "./QuickUndoToast";
import { ReviewQueueHeaderActions } from "./ReviewQueueHeaderActions";
import { ReviewQueueList } from "./ReviewQueueList";
import { ReviewQueueDetail, ReviewQueueDetailEmpty } from "./ReviewQueueDetail";
import { ReviewQueuePredictionControls } from "./ReviewQueuePredictionControls";
import { ReviewQueuePredictionExplanation } from "./ReviewQueuePredictionExplanation";
import {
  ReviewQueueToolbar,
  type ReviewQueueFilterValue,
} from "./ReviewQueueToolbar";
import { ReviewQueueVerdictControls } from "./ReviewQueueVerdictControls";
import { predictionVerdictPresentation } from "./PredictionVerdictIndicator";
import { useReviewQueueNavigationShortcuts } from "./review-queue-shortcuts";
import {
  getAdjacentTraceIds,
  getBaselineStatus,
  getReviewQueuePageIndex,
  reviewQueueItemToTraceEntry,
  truncateTraceId,
  type BaselineStatus,
} from "./review-queue-utils";

const EMPTY_QUEUE_AUTO_LOAD_LIMIT = 3;
const EMPTY_REVIEW_QUEUE_ITEMS: ReviewQueueItem[] = [];

function markedLabel(verdict: DatasetJudgmentVerdict) {
  const { label } = predictionVerdictPresentation(verdict);
  return `Marked as ${label.toLowerCase()}`;
}

type ActiveJudgment = {
  traceId: string;
  verdict: DatasetJudgmentVerdict;
  item?: ReviewQueueItem;
  pageIndex?: number;
  nextTraceId?: string | null;
  nextReviewQueueItem?: ReviewQueueItem;
  initialCriteriaKeys?: string[];
};

type ReviewQueuePanelAction = "none" | "open" | "sync";

type JudgingRun = {
  predictionCount: number;
  completedBeforeRun: number;
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
  const [queueFilter, setQueueFilter] =
    useState<ReviewQueueFilterValue>("all");
  const [allQueueFullyJudged, setAllQueueFullyJudged] = useState(false);
  const predictionFilter: ReviewQueuePredictionFilter | undefined =
    queueFilter === "all" ? undefined : queueFilter;
  const {
    data: predictionStatus,
    isError: predictionStatusIsError,
    isLoading: predictionStatusIsLoading,
  } = useReviewQueuePredictionStatus(deploymentId);
  const {
    data,
    isLoading,
    isError,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useDatasetReviewQueue(deploymentId, true, predictionFilter);
  const avatarBust = useDeploymentAvatarBust(deploymentId);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [predictionExplanationTraceId, setPredictionExplanationTraceId] =
    useState<string | null>(null);
  const [activeJudgment, setActiveJudgment] = useState<ActiveJudgment | null>(null);
  const [judgingRun, setJudgingRun] = useState<JudgingRun | null>(null);
  // Mirrors selectedId for synchronous reads inside mutation callbacks.
  const selectedIdRef = useRef<string | null>(null);
  // Tracks the trace currently shown in the open detail panel.
  const syncedPanelTraceIdRef = useRef<string | null>(null);
  const queueListRef = useRef<HTMLUListElement | null>(null);
  const emptyQueueAutoLoadCountRef = useRef(0);
  const items = useMemo(
    () => data?.pages.flatMap((page) => page.items) ?? EMPTY_REVIEW_QUEUE_ITEMS,
    [data?.pages],
  );
  const judgingCount =
    (predictionStatus?.queued ?? 0) + (predictionStatus?.in_progress ?? 0);
  const hasActivePredictions = judgingCount > 0;
  const hasVisibleUnjudgedItems = items.some((item) => !item.prediction);
  const currentAllQueueFullyJudged =
    predictionFilter === undefined &&
    data !== undefined &&
    !hasNextPage &&
    !hasVisibleUnjudgedItems;
  const filteredQueueFullyJudged =
    predictionFilter !== undefined &&
    allQueueFullyJudged &&
    !hasVisibleUnjudgedItems;
  const autoJudgeLoading = isLoading || predictionStatusIsLoading;
  const autoJudgeState = autoJudgeLoading
    ? "loading"
    : hasActivePredictions
      ? "judging"
      : currentAllQueueFullyJudged || filteredQueueFullyJudged
        ? "nothing-to-judge"
        : "ready";
  const loadedPageCount = data?.pages.length ?? 0;
  const selectedItem =
    items.find((item) => item.trace_id === selectedId) ?? items[0] ?? null;
  const selectedPrediction = selectedItem?.prediction ?? null;
  const selectedPredictionFailed =
    selectedItem?.prediction_status === "failed" && !selectedPrediction;
  const selectedIndex = selectedItem
    ? items.findIndex((item) => item.trace_id === selectedItem.trace_id)
    : -1;
  const baselineStatus = getBaselineStatus(summary);
  const canLoadMore = Boolean(hasNextPage);

  useEffect(() => {
    if (predictionFilter !== undefined || data === undefined) {
      return;
    }
    setAllQueueFullyJudged(!hasNextPage && !hasVisibleUnjudgedItems);
  }, [data, hasNextPage, hasVisibleUnjudgedItems, predictionFilter]);

  useEffect(() => {
    if (
      judgingRun === null ||
      predictionStatusIsError ||
      predictionStatus === undefined ||
      judgingCount > 0
    ) {
      return;
    }

    const completedCount = Math.min(
      judgingRun.predictionCount,
      Math.max(0, predictionStatus.completed - judgingRun.completedBeforeRun),
    );
    const failedCount = judgingRun.predictionCount - completedCount;
    const toastOptions = {
      closeButton: true,
      description:
        failedCount === 0
          ? "Traces scored by the judge are ready to review."
          : completedCount > 0
            ? "Retry them on the next run or select a verdict."
            : "Predictions could not be generated. Retry them on the next run.",
    };
    if (completedCount === 0) {
      toast.error("Assessment failed", toastOptions);
    } else if (failedCount > 0) {
      toast.warning("Some traces couldn’t be judged", toastOptions);
    } else {
      toast.success("Assessment complete", toastOptions);
    }
    setJudgingRun(null);
  }, [
    judgingCount,
    judgingRun,
    predictionStatus,
    predictionStatusIsError,
  ]);

  const activeVerdict =
    activeJudgment && selectedItem && activeJudgment.traceId === selectedItem.trace_id
      ? activeJudgment.verdict
      : null;

  const selectTraceId = useCallback((traceId: string | null) => {
    setPredictionExplanationTraceId(null);
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

  const removeQueueItem = useRemoveReviewQueueItem(
    deploymentId,
    predictionFilter,
  );
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
        initialCriteriaKeys: variables.initialCriteriaKeys,
      };
      setActiveJudgment(judgment);
      if (verdictHasCriteria(variables.verdict)) {
        return;
      }
      commitJudgment(judgment);
    },
  });

  const undoJudgment = useUndoDatasetJudgment(deploymentId, predictionFilter);
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
    trigger: HTMLElement | null,
    initialCriteriaKeys?: string[],
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
      initialCriteriaKeys,
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
  const handleNavigate = (index: number) => {
    const item = items[index];
    if (item) {
      handleSelectTrace(item.trace_id);
    }
  };
  const canGoPrevious = selectedIndex > 0;
  const canGoNext = selectedIndex >= 0 && selectedIndex < items.length - 1;
  const goPrevious = () => handleNavigate(selectedIndex - 1);
  const goNext = () => handleNavigate(selectedIndex + 1);

  const navigateFromOutsideList = (navigate: () => void) => () => {
    navigate();
    queueListRef.current?.focus({ preventScroll: true });
  };

  useReviewQueueNavigationShortcuts({
    disabled: Boolean(
      activeJudgment && verdictHasCriteria(activeJudgment.verdict),
    ),
    onPrevious: navigateFromOutsideList(goPrevious),
    onNext: navigateFromOutsideList(goNext),
  });

  return (
    <>
      <EvalTabCard className="@container/review-card min-h-0">
        <EvalTabCardHeader
          label="Review traces"
          description="Confirm the judge's verdict or select your own"
          className="px-4 py-4 @[520px]/review-card:px-6"
        >
          {baselineStatus && <BaselineStatusBadge status={baselineStatus} />}
          {selectedItem && (
            <ReviewQueueHeaderActions
              position={selectedIndex + 1}
              total={items.length}
              canGoPrevious={canGoPrevious}
              canGoNext={canGoNext}
              onPrevious={goPrevious}
              onNext={goNext}
              traceLabel={truncateTraceId(selectedItem.trace_id)}
              onOpenTrace={
                onOpenTrace
                  ? () => applyReviewQueueSelection(selectedItem, "open")
                  : undefined
              }
            />
          )}
        </EvalTabCardHeader>
        <EvalTabCardBody className="flex-col @[760px]/review-card:flex-row">
          <div className="flex w-full flex-none flex-col bg-card dark:bg-surface @[760px]/review-card:w-[clamp(18rem,34%,24.5rem)] @[760px]/review-card:border-r @[760px]/review-card:border-border">
            <ReviewQueueToolbar
              deploymentId={deploymentId}
              account={account}
              autoJudgeState={autoJudgeState}
              judgingCount={judgingCount}
              onJudgingStarted={(predictionCount) =>
                setJudgingRun({
                  predictionCount,
                  completedBeforeRun: predictionStatus?.completed ?? 0,
                })
              }
              filter={queueFilter}
              onFilterChange={setQueueFilter}
            />
            <aside className="flex max-h-64 min-h-0 flex-1 flex-col overflow-y-auto border-b border-border @[760px]/review-card:max-h-none @[760px]/review-card:border-b-0">
              <ReviewQueueList
                listRef={queueListRef}
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
          </div>

          <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background">
            {selectedItem && (
              <div
                data-review-queue-controls
                className={cn(
                  "flex flex-none items-center bg-card px-4 py-3 dark:bg-surface @[520px]/review-card:px-6",
                  !selectedPredictionFailed && "border-b border-border",
                )}
              >
                {selectedPrediction ? (
                  <ReviewQueuePredictionControls
                    key={selectedItem.trace_id}
                    prediction={selectedPrediction}
                    isPending={postJudgment.isPending}
                    activeVerdict={activeVerdict}
                    showError={postJudgment.isError || undoJudgment.isError}
                    explanationOpen={
                      predictionExplanationTraceId === selectedItem.trace_id
                    }
                    onExplanationOpenChange={(open) =>
                      setPredictionExplanationTraceId(
                        open ? selectedItem.trace_id : null,
                      )
                    }
                    onSelect={(verdict, trigger, agreesWithJudge) =>
                      handleJudgeTrace(
                        selectedItem.trace_id,
                        verdict,
                        trigger,
                        agreesWithJudge && verdictHasCriteria(verdict)
                          ? predictedCriterionKeysForVerdict(
                              selectedPrediction.criteria,
                              verdict,
                            )
                          : undefined,
                      )
                    }
                  />
                ) : (
                  <ReviewQueueVerdictControls
                    isPending={postJudgment.isPending}
                    activeVerdict={activeVerdict}
                    showError={postJudgment.isError || undoJudgment.isError}
                    onSelect={(verdict, trigger) =>
                      handleJudgeTrace(selectedItem.trace_id, verdict, trigger)
                    }
                  />
                )}
              </div>
            )}
            {selectedPredictionFailed && (
              <div className="flex-none border-b border-border bg-card px-4 pb-3 pt-0 dark:bg-surface @[520px]/review-card:px-6">
                <WarningPanel title="Couldn’t judge" variant="inline" size="xs">
                  No prediction was made. It’ll re-run next time you run the judge.
                </WarningPanel>
              </div>
            )}
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
              <>
                {selectedPrediction &&
                  predictionExplanationTraceId === selectedItem.trace_id && (
                    <div className="dp-scroll max-h-[min(32rem,calc(100vh-12rem))] flex-none overflow-y-auto border-b border-border bg-card px-4 pb-4 dark:bg-surface @[520px]/review-card:px-6">
                      <ReviewQueuePredictionExplanation
                        prediction={selectedPrediction}
                      />
                    </div>
                  )}
                <ReviewQueueDetail
                  item={selectedItem}
                  account={account}
                  agentName={agentName}
                  agentLabel={agentLabel}
                  agentAvatarUrl={resolvedAgentAvatarUrl}
                />
              </>
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
            initialKeys={activeJudgment.initialCriteriaKeys}
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
