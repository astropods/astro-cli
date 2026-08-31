import { useCallback, useEffect, useRef } from "react";
import {
  keepPreviousData,
  type InfiniteData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useApiClient } from "../../lib/api-context";
import type {
  DatasetEvaluationsResponse,
  DatasetItemOutputsResponse,
  DatasetItemResponse,
  EvalDatasetItemsResponse,
  EvalDatasetResponse,
  EvaluationRun,
  EvaluationSetResponse,
  EvaluationStatusCounts,
  EvaluatorOutputValue,
  ReviewQueueItem,
  ReviewQueueEvaluationFilter,
  TraceEvaluationResponse,
  ReviewQueueResponse,
} from "@/lib/api";
import { evalKeys } from "./keys";

type AddDatasetItemVariables = {
  traceId: string;
  evaluationRunId?: string;
  outputs: EvaluatorOutputValue[];
};

type RemoveDatasetItemVariables = {
  traceId: string;
  reviewQueueItem?: ReviewQueueItem;
  reviewQueuePageIndex?: number;
};

type DatasetItemOutputsVariables = {
  traceId: string;
  outputs: EvaluatorOutputValue[];
};

type ReviewQueuePageParam = string | undefined;

type ReviewQueueInfiniteData = InfiniteData<
  ReviewQueueResponse,
  ReviewQueuePageParam
>;

const REVIEW_QUEUE_PAGE_SIZE = 50;
const REVIEW_QUEUE_POLL_INTERVAL_MS = 5_000;

export function useAgentEvaluationSet(account: string, name: string) {
  const api = useApiClient();
  return useQuery({
    queryKey: evalKeys.evaluationSet(account, name),
    queryFn: (): Promise<EvaluationSetResponse> =>
      api.getAgentEvaluationSet(account, name),
    enabled: !!account && !!name,
    staleTime: 5 * 60_000,
  });
}

export function useEvalDataset(deploymentId: string) {
  const api = useApiClient();
  return useQuery({
    queryKey: evalKeys.summary(deploymentId),
    queryFn: (): Promise<EvalDatasetResponse> => api.getEvalDataset(deploymentId),
    enabled: !!deploymentId,
    staleTime: 60_000,
  });
}

export function useEvalDatasetItems(
  deploymentId: string,
  limit = 50,
  enabled = true,
) {
  const api = useApiClient();
  return useInfiniteQuery({
    queryKey: evalKeys.items(deploymentId, limit),
    queryFn: ({ pageParam }): Promise<EvalDatasetItemsResponse> =>
      api.getEvalDatasetItems(deploymentId, { page: pageParam, limit }),
    initialPageParam: 1,
    getNextPageParam: (last) =>
      last.page < last.total_pages ? last.page + 1 : undefined,
    enabled: !!deploymentId && enabled,
    staleTime: 60_000,
  });
}

export function useDatasetReviewQueue(
  deploymentId: string,
  enabled = true,
  evaluation?: ReviewQueueEvaluationFilter,
) {
  const api = useApiClient();
  return useInfiniteQuery({
    queryKey: evalKeys.reviewQueue(deploymentId, evaluation),
    queryFn: ({
      pageParam,
    }: {
      pageParam: ReviewQueuePageParam;
    }): Promise<ReviewQueueResponse> =>
      api.getDatasetReviewQueue(deploymentId, {
        limit: REVIEW_QUEUE_PAGE_SIZE,
        cursor: pageParam,
        evaluation,
      }),
    initialPageParam: undefined as ReviewQueuePageParam,
    getNextPageParam: (last): ReviewQueuePageParam =>
      last.next_cursor || undefined,
    enabled: !!deploymentId && enabled,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  });
}

function hasActiveEvaluations(status: EvaluationStatusCounts | undefined) {
  return status !== undefined && status.queued + status.in_progress > 0;
}

function useDatasetEvaluationStatus(
  deploymentId: string,
  enabled = true,
) {
  const api = useApiClient();

  return useQuery({
    queryKey: evalKeys.evaluationStatus(deploymentId),
    queryFn: (): Promise<EvaluationStatusCounts> =>
      api.getDatasetEvaluationStatus(deploymentId),
    enabled: !!deploymentId && enabled,
    refetchInterval: (query) =>
      hasActiveEvaluations(query.state.data)
        ? REVIEW_QUEUE_POLL_INTERVAL_MS
        : false,
  });
}

export function isRunActive(run: EvaluationRun | null | undefined) {
  return run?.status === "queued" || run?.status === "in_progress";
}

export function useTraceEvaluation(
  deploymentId: string,
  traceId: string | undefined,
  evaluating = false,
) {
  const api = useApiClient();

  return useQuery({
    queryKey: evalKeys.traceEvaluation(deploymentId, traceId ?? ""),
    queryFn: (): Promise<TraceEvaluationResponse> =>
      api.getTraceEvaluation(deploymentId, traceId as string),
    enabled: !!deploymentId && !!traceId,
    refetchInterval: (query) =>
      evaluating || isRunActive(query.state.data?.run)
        ? REVIEW_QUEUE_POLL_INTERVAL_MS
        : false,
  });
}

export function useReviewQueueEvaluationStatus(
  deploymentId: string,
  enabled = true,
) {
  const queryClient = useQueryClient();
  const query = useDatasetEvaluationStatus(deploymentId, enabled);
  const active = hasActiveEvaluations(query.data);
  const previousActivityRef = useRef<{
    deploymentId: string;
    active: boolean;
  } | null>(null);

  useEffect(() => {
    if (query.dataUpdatedAt === 0) {
      return;
    }

    const previous =
      previousActivityRef.current?.deploymentId === deploymentId
        ? previousActivityRef.current.active
        : undefined;
    previousActivityRef.current = { deploymentId, active };

    // The initial queue request already runs alongside the first status read.
    // Subsequent active reads refresh the selected queue, including the final
    // transition to inactive, while leaving other filter caches merely stale.
    if (previous !== undefined && (previous || active)) {
      void queryClient.invalidateQueries({
        queryKey: evalKeys.reviewQueues(deploymentId),
      });
    }

    // Only the settling read can leave a cached trace reading as empty, and the
    // selected trace polls its own results, so marking the rest stale here
    // keeps revisits correct without expiring the cache on every active read.
    if (previous && !active) {
      void queryClient.invalidateQueries({
        queryKey: evalKeys.traceEvaluations(deploymentId),
        refetchType: "none",
      });
    }
  }, [active, deploymentId, query.dataUpdatedAt, queryClient]);

  return query;
}

export function usePostDatasetEvaluations(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<DatasetEvaluationsResponse, Error, void>({
    mutationFn: () => api.postDatasetEvaluations(deploymentId),
    onSuccess: async (response) => {
      const enqueuedTraceIds = new Set(response.enqueued_trace_ids);
      queryClient.setQueriesData<ReviewQueueInfiniteData>(
        { queryKey: evalKeys.reviewQueues(deploymentId) },
        (old) => markReviewQueueItemsQueued(old, enqueuedTraceIds),
      );

      await queryClient.invalidateQueries({
        queryKey: evalKeys.evaluationStatus(deploymentId),
      });
    },
  });
}

function markReviewQueueItemsQueued(
  data: ReviewQueueInfiniteData | undefined,
  traceIds: ReadonlySet<string>,
) {
  if (!data || traceIds.size === 0) {
    return data;
  }

  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      items: page.items.map((item) =>
        traceIds.has(item.trace_id)
          ? {
              ...item,
              run: { status: "queued" as const, error: null },
            }
          : item,
      ),
    })),
  };
}

export function useAddDatasetItem(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<DatasetItemResponse, Error, AddDatasetItemVariables>({
    mutationFn: ({ traceId, evaluationRunId, outputs }) =>
      api.postDatasetItem(deploymentId, {
        trace_id: traceId,
        evaluation_run_id: evaluationRunId,
        evaluator_outputs: outputs,
      }),
    onSuccess: async () => {
      // Queue removal is the caller's job (useRemoveReviewQueueItem) so an added
      // trace can stay visible until its undo window closes.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ]);
    },
  });
}

/** Removes a trace from the review-queue cache. Callers own the timing so an
 *  added trace can stay visible until the reviewer dismisses its panel. */
export function useRemoveReviewQueueItem(
  deploymentId: string,
  evaluation?: ReviewQueueEvaluationFilter,
) {
  const queryClient = useQueryClient();
  return useCallback(
    (traceId: string) => {
      queryClient.setQueryData<ReviewQueueInfiniteData>(
        evalKeys.reviewQueue(deploymentId, evaluation),
        (old) => removeReviewQueueItem(old, traceId),
      );
    },
    [queryClient, deploymentId, evaluation],
  );
}

function insertReviewQueueItem(
  items: ReviewQueueItem[],
  restored: ReviewQueueItem,
) {
  return [restored, ...items.filter((item) => item.trace_id !== restored.trace_id)]
    .sort((a, b) => b.timestamp.localeCompare(a.timestamp));
}

function removeReviewQueueItem(
  data: ReviewQueueInfiniteData | undefined,
  traceId: string,
) {
  if (!data) {
    return data;
  }

  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      items: page.items.filter((item) => item.trace_id !== traceId),
    })),
  };
}

function insertReviewQueueItemPage(
  data: ReviewQueueInfiniteData | undefined,
  restored: ReviewQueueItem,
  pageIndex = 0,
) {
  if (!data || data.pages.length === 0) {
    return data;
  }

  const targetPageIndex =
    pageIndex >= 0 && pageIndex < data.pages.length ? pageIndex : 0;

  return {
    ...data,
    pages: data.pages.map((page, index) =>
      index === targetPageIndex
        ? {
            ...page,
            items: insertReviewQueueItem(page.items, restored),
          }
        : {
            ...page,
            items: page.items.filter(
              (item) => item.trace_id !== restored.trace_id,
            ),
          },
    ),
  };
}

export function useRemoveDatasetItem(
  deploymentId: string,
  evaluation?: ReviewQueueEvaluationFilter,
) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<DatasetItemResponse, Error, RemoveDatasetItemVariables>({
    mutationFn: ({ traceId }) => api.deleteDatasetItem(deploymentId, traceId),
    onSuccess: async (_data, variables) => {
      const restoredItem = variables.reviewQueueItem;
      if (restoredItem) {
        queryClient.setQueryData<ReviewQueueInfiniteData>(
          evalKeys.reviewQueue(deploymentId, evaluation),
          (old) =>
            insertReviewQueueItemPage(
              old,
              restoredItem,
              variables.reviewQueuePageIndex,
            ),
        );
      }

      const invalidations = [
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ];
      if (!restoredItem) {
        invalidations.push(
          queryClient.invalidateQueries({
            queryKey: evalKeys.reviewQueues(deploymentId),
          }),
        );
      }

      // Queue-originated removal stays optimistic because it already has the
      // full item. Dataset-originated removal reloads the server-owned queue.
      await Promise.all(invalidations);
    },
  });
}

export function useSetDatasetItemOutputs(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<
    DatasetItemOutputsResponse,
    Error,
    DatasetItemOutputsVariables
  >({
    mutationFn: ({ traceId, outputs }) =>
      api.putDatasetItemEvaluatorOutputs(deploymentId, traceId, {
        values: outputs,
      }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ]);
    },
  });
}
