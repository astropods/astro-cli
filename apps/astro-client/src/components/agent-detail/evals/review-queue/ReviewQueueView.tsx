import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { toast } from "sonner";
import { Spinner } from "@/components/ui/spinner";
import { getDeploymentAvatarUrl } from "@/lib/assets";
import { useDeploymentAvatarBust } from "@/lib/avatar-bust";
import {
  isRunActive,
  useAddDatasetItem,
  useAgentEvaluationSet,
  useDatasetReviewQueue,
  useRemoveDatasetItem,
  useRemoveReviewQueueItem,
  useReviewQueueEvaluationStatus,
  useTraceEvaluation,
} from "@/api/queries/evals";
import type {
  EvaluatorOutputValue,
  ReviewQueueItem,
  ReviewQueueEvaluationFilter,
  TraceEntry,
  TraceEvaluatorResult,
} from "@/lib/api";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "../EvalTabCard";
import { flyTraceToDataset } from "../review-queue-motion";
import { QuickUndoToast } from "./QuickUndoToast";
import { ReviewQueueHeaderActions } from "./ReviewQueueHeaderActions";
import { ReviewQueueList } from "./ReviewQueueList";
import { ReviewQueueDetail, ReviewQueueDetailEmpty } from "./ReviewQueueDetail";
import { ReviewQueueEvaluationSection } from "./ReviewQueueEvaluationSection";
import {
  ReviewQueueToolbar,
  type ReviewQueueFilterValue,
} from "./ReviewQueueToolbar";
import { useReviewQueueNavigationShortcuts } from "./review-queue-shortcuts";
import {
  getAdjacentTraceIds,
  getReviewQueuePageIndex,
  reviewQueueItemToTraceEntry,
  truncateTraceId,
} from "./review-queue-utils";

const EMPTY_QUEUE_AUTO_LOAD_LIMIT = 3;
const EMPTY_REVIEW_QUEUE_ITEMS: ReviewQueueItem[] = [];
const EMPTY_EVALUATOR_RESULTS: TraceEvaluatorResult[] = [];

type AddedItem = {
  traceId: string;
  item?: ReviewQueueItem;
  pageIndex?: number;
};

type NextSelection = {
  traceId: string;
  nextTraceId?: string | null;
  nextReviewQueueItem?: ReviewQueueItem;
};

type ReviewQueuePanelAction = "none" | "open" | "sync";

type EvaluationRun = {
  predictionCount: number;
  completedBeforeRun: number;
};

export interface ReviewQueueViewProps {
  deploymentId: string;
  account: string;
  agentName: string;
  agentLabel: string;
  agentAvatarUrl?: string;
  datasetTargetRef?: RefObject<HTMLSpanElement | null>;
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
  datasetTargetRef,
  onOpenTrace,
  onSelectedTraceChange,
  onSelectedTraceCleared,
}: ReviewQueueViewProps) {
  const [queueFilter, setQueueFilter] =
    useState<ReviewQueueFilterValue>("all");
  const [allQueueFullyEvaluated, setAllQueueFullyEvaluated] = useState(false);
  const evaluationFilter: ReviewQueueEvaluationFilter | undefined =
    queueFilter === "all" ? undefined : queueFilter;
  const {
    data: evaluationStatus,
    isError: evaluationStatusIsError,
    isLoading: evaluationStatusIsLoading,
  } = useReviewQueueEvaluationStatus(deploymentId);
  const {
    data,
    isLoading,
    isError,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useDatasetReviewQueue(deploymentId, true, evaluationFilter);
  const avatarBust = useDeploymentAvatarBust(deploymentId);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  // Results open themselves once a run has verdicts to show; this records a
  // trace whose panel the reader has since opened or closed by hand.
  const [resultsOverride, setResultsOverride] = useState<{
    traceId: string;
    open: boolean;
  } | null>(null);
  const [addedItem, setAddedItem] = useState<AddedItem | null>(null);
  const [evaluationRun, setEvaluationRun] = useState<EvaluationRun | null>(null);
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
  const evaluatingCount =
    (evaluationStatus?.queued ?? 0) + (evaluationStatus?.in_progress ?? 0);
  const hasActiveEvaluations = evaluatingCount > 0;
  const hasVisibleUnevaluatedItems = items.some(
    (item) => item.run?.status !== "completed",
  );
  const currentAllQueueFullyEvaluated =
    evaluationFilter === undefined &&
    data !== undefined &&
    !hasNextPage &&
    !hasVisibleUnevaluatedItems;
  const filteredQueueFullyEvaluated =
    evaluationFilter !== undefined &&
    allQueueFullyEvaluated &&
    !hasVisibleUnevaluatedItems;
  const autoEvaluateLoading = isLoading || evaluationStatusIsLoading;
  const autoEvaluateState = autoEvaluateLoading
    ? "loading"
    : hasActiveEvaluations
      ? "evaluating"
      : currentAllQueueFullyEvaluated || filteredQueueFullyEvaluated
        ? "nothing-to-evaluate"
        : "ready";
  const loadedPageCount = data?.pages.length ?? 0;
  const selectedItem =
    items.find((item) => item.trace_id === selectedId) ?? items[0] ?? null;
  const selectedEvaluating = isRunActive(selectedItem?.run);
  const { data: selectedEvaluation, isLoading: evaluationLoading } =
    useTraceEvaluation(
      deploymentId,
      selectedItem?.trace_id,
      selectedEvaluating,
    );
  const selectedResults =
    selectedEvaluation?.evaluators ?? EMPTY_EVALUATOR_RESULTS;
  const selectedHasEvaluation = Boolean(selectedItem?.run);
  const selectedEvaluationPending = selectedEvaluating || evaluationLoading;
  const selectedScoredCount = selectedResults.filter(
    (result) => result.status === "completed",
  ).length;
  const selectedScored =
    !selectedEvaluationPending && selectedScoredCount > 0;
  const resultsOpen =
    resultsOverride && resultsOverride.traceId === selectedItem?.trace_id
      ? resultsOverride.open
      : selectedHasEvaluation && !selectedEvaluationPending;
  const selectedIndex = selectedItem
    ? items.findIndex((item) => item.trace_id === selectedItem.trace_id)
    : -1;
  const canLoadMore = Boolean(hasNextPage);

  useEffect(() => {
    if (evaluationFilter !== undefined || data === undefined) {
      return;
    }
    setAllQueueFullyEvaluated(!hasNextPage && !hasVisibleUnevaluatedItems);
  }, [data, hasNextPage, hasVisibleUnevaluatedItems, evaluationFilter]);

  useEffect(() => {
    if (
      evaluationRun === null ||
      evaluationStatusIsError ||
      evaluationStatus === undefined ||
      evaluatingCount > 0
    ) {
      return;
    }

    const completedCount = Math.min(
      evaluationRun.predictionCount,
      Math.max(0, evaluationStatus.completed - evaluationRun.completedBeforeRun),
    );
    const failedCount = evaluationRun.predictionCount - completedCount;
    const toastOptions = {
      closeButton: true,
      description:
        failedCount === 0
          ? "Traces scored by the evaluator are ready to review."
          : completedCount > 0
            ? "Retry them on the next run or review the traces manually."
            : "Evaluations could not be generated. Retry them on the next run.",
    };
    if (completedCount === 0) {
      toast.error("Assessment failed", toastOptions);
    } else if (failedCount > 0) {
      toast.warning("Some traces couldn’t be evaluated", toastOptions);
    } else {
      toast.success("Assessment complete", toastOptions);
    }
    setEvaluationRun(null);
  }, [
    evaluatingCount,
    evaluationRun,
    evaluationStatus,
    evaluationStatusIsError,
  ]);

  const selectTraceId = useCallback((traceId: string | null) => {
    setResultsOverride(null);
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
    evaluationFilter,
  );
  const commitAdd = useCallback(
    (added: NextSelection) => {
      removeQueueItem(added.traceId);
      if (selectedIdRef.current !== added.traceId) {
        return;
      }
      selectTraceId(added.nextTraceId ?? null);
      if (added.nextReviewQueueItem) {
        applyReviewQueueSelection(added.nextReviewQueueItem, "sync");
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

  const addItem = useAddDatasetItem(deploymentId);
  const removeItem = useRemoveDatasetItem(deploymentId, evaluationFilter);
  const { data: evaluationSet } = useAgentEvaluationSet(account, agentName);
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
    addItem.reset();
    removeItem.reset();
    const item = items.find((candidate) => candidate.trace_id === traceId);
    if (item) {
      applyReviewQueueSelection(item, "sync");
    } else {
      selectTraceId(traceId);
    }
  };

  const handleAdd = (
    outputs: EvaluatorOutputValue[],
    trigger: HTMLElement | null,
  ) => {
    if (!selectedItem) {
      return;
    }

    const traceId = selectedItem.trace_id;
    const { previousTraceId, nextTraceId } = getAdjacentTraceIds(items, traceId);
    const nextSelectedTraceId = nextTraceId ?? previousTraceId;
    const pageIndex = getReviewQueuePageIndex(data?.pages, traceId);
    const nextSelection: NextSelection = {
      traceId,
      nextTraceId: nextSelectedTraceId,
      nextReviewQueueItem: nextSelectedTraceId
        ? items.find((item) => item.trace_id === nextSelectedTraceId)
        : undefined,
    };
    const triggerRect = trigger?.getBoundingClientRect() ?? null;

    addItem.mutate(
      {
        traceId,
        evaluationRunId: selectedEvaluation?.run?.id,
        outputs,
      },
      {
        onSuccess: () => {
          flyTraceToDataset(triggerRect, datasetTargetRef?.current);
          commitAdd(nextSelection);
          setAddedItem({ traceId, item: selectedItem, pageIndex });
        },
      },
    );
  };

  const handleUndo = () => {
    if (!addedItem) {
      return;
    }

    const { traceId, item, pageIndex } = addedItem;
    removeItem.reset();
    removeItem.mutate(
      { traceId, reviewQueueItem: item, reviewQueuePageIndex: pageIndex },
      {
        onSuccess: () => {
          setAddedItem(null);
          if (item) {
            applyReviewQueueSelection(item, "sync");
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
    disabled: addItem.isPending,
    onPrevious: navigateFromOutsideList(goPrevious),
    onNext: navigateFromOutsideList(goNext),
  });

  return (
    <>
      <EvalTabCard className="@container/review-card min-h-0">
        <EvalTabCardHeader
          label="Review traces"
          description="Review traces from the last 30 days."
          className="px-4 py-4 @[520px]/review-card:px-6"
        >
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
              autoEvaluateState={autoEvaluateState}
              evaluatingCount={evaluatingCount}
              onEvaluationStarted={(predictionCount) =>
                setEvaluationRun({
                  predictionCount,
                  completedBeforeRun: evaluationStatus?.completed ?? 0,
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
              <ReviewQueueEvaluationSection
                key={selectedItem.trace_id}
                evaluators={evaluationSet?.evaluators ?? []}
                results={selectedResults}
                scored={selectedScored}
                attempted={selectedHasEvaluation}
                open={resultsOpen}
                onOpenChange={(open: boolean) =>
                  setResultsOverride({ traceId: selectedItem.trace_id, open })
                }
                loading={selectedEvaluationPending}
                isSaving={addItem.isPending}
                addError={
                  addItem.isError
                    ? "Could not add to the dataset. Try again."
                    : removeItem.isError
                      ? "Could not update the review queue. Try again."
                      : undefined
                }
                onAdd={handleAdd}
              />
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
                <ReviewQueueDetail
                  item={selectedItem}
                  evaluation={selectedEvaluation}
                  account={account}
                  agentName={agentName}
                  agentLabel={agentLabel}
                  agentAvatarUrl={resolvedAgentAvatarUrl}
                />
              </>
            ) : (
              <ReviewQueueDetailEmpty
                showActionError={addItem.isError || removeItem.isError}
                canLoadMore={canLoadMore}
                isLoadingMore={isFetchingNextPage}
                onLoadMore={handleLoadMore}
              />
            )}
          </div>
        </EvalTabCardBody>
      </EvalTabCard>
      {addedItem && (
        <QuickUndoToast
          key={addedItem.traceId}
          label="Added to dataset"
          isUndoing={removeItem.isPending}
          onUndo={handleUndo}
          onDismiss={() => setAddedItem(null)}
        />
      )}
    </>
  );
}
