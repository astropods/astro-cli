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
  DatasetJudgmentCriteriaResponse,
  DatasetPredictionsResponse,
  DatasetJudgmentResponse,
  DatasetJudgmentVerdict,
  EvalDatasetItemsResponse,
  EvalDatasetResponse,
  JudgmentCriterion,
  PredictionStatusCounts,
  ReviewQueueItem,
  ReviewQueuePredictionFilter,
  ReviewQueueResponse,
} from "@/lib/api";
import { evalKeys } from "./keys";

type DatasetJudgmentVariables = {
  traceId: string;
  verdict: DatasetJudgmentVerdict;
  nextTraceId?: string | null;
  reviewQueueItem?: ReviewQueueItem;
  nextReviewQueueItem?: ReviewQueueItem;
  reviewQueuePageIndex?: number;
  initialCriteriaKeys?: string[];
};

type DatasetUndoJudgmentVariables = {
  traceId: string;
  reviewQueueItem?: ReviewQueueItem;
  reviewQueuePageIndex?: number;
};

type DatasetJudgmentCriteriaVariables = {
  traceId: string;
  criteria: JudgmentCriterion[];
};

type ReviewQueuePageParam = string | undefined;

type ReviewQueueInfiniteData = InfiniteData<
  ReviewQueueResponse,
  ReviewQueuePageParam
>;

const REVIEW_QUEUE_PAGE_SIZE = 50;
const REVIEW_QUEUE_POLL_INTERVAL_MS = 5_000;

interface UsePostDatasetJudgmentOptions {
  onSuccess?: (
    data: DatasetJudgmentResponse,
    variables: DatasetJudgmentVariables,
  ) => void;
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
  prediction?: ReviewQueuePredictionFilter,
) {
  const api = useApiClient();
  return useInfiniteQuery({
    queryKey: evalKeys.reviewQueue(deploymentId, prediction),
    queryFn: ({
      pageParam,
    }: {
      pageParam: ReviewQueuePageParam;
    }): Promise<ReviewQueueResponse> =>
      api.getDatasetReviewQueue(deploymentId, {
        limit: REVIEW_QUEUE_PAGE_SIZE,
        cursor: pageParam,
        prediction,
      }),
    initialPageParam: undefined as ReviewQueuePageParam,
    getNextPageParam: (last): ReviewQueuePageParam =>
      last.next_cursor || undefined,
    enabled: !!deploymentId && enabled,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  });
}

function hasActivePredictions(status: PredictionStatusCounts | undefined) {
  return status !== undefined && status.queued + status.in_progress > 0;
}

export function useDatasetPredictionStatus(
  deploymentId: string,
  enabled = true,
) {
  const api = useApiClient();

  return useQuery({
    queryKey: evalKeys.predictionStatus(deploymentId),
    queryFn: (): Promise<PredictionStatusCounts> =>
      api.getDatasetPredictionStatus(deploymentId),
    enabled: !!deploymentId && enabled,
    refetchInterval: (query) =>
      hasActivePredictions(query.state.data)
        ? REVIEW_QUEUE_POLL_INTERVAL_MS
        : false,
  });
}

export function useReviewQueuePredictionStatus(
  deploymentId: string,
  enabled = true,
) {
  const queryClient = useQueryClient();
  const query = useDatasetPredictionStatus(deploymentId, enabled);
  const active = hasActivePredictions(query.data);
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
  }, [active, deploymentId, query.dataUpdatedAt, queryClient]);

  return query;
}

export function usePostDatasetPredictions(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<DatasetPredictionsResponse, Error, void>({
    mutationFn: () => api.postDatasetPredictions(deploymentId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: evalKeys.reviewQueues(deploymentId),
        }),
        queryClient.invalidateQueries({
          queryKey: evalKeys.predictionStatus(deploymentId),
        }),
      ]);
    },
  });
}

export function usePostDatasetJudgment(
  deploymentId: string,
  options: UsePostDatasetJudgmentOptions = {},
) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<
    DatasetJudgmentResponse,
    Error,
    DatasetJudgmentVariables
  >({
    mutationFn: ({ traceId, verdict }) =>
      api.postDatasetJudgment(deploymentId, {
        trace_id: traceId,
        verdict,
      }),
    onSuccess: async (data, variables) => {
      options.onSuccess?.(data, variables);

      // Queue removal is the caller's job (useRemoveReviewQueueItem) so good/bad
      // can keep the trace visible until Done.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ]);
    },
  });
}

/** Removes a trace from the review-queue cache. Callers own the timing so a
 *  judged trace can stay visible until the reviewer dismisses its panel. */
export function useRemoveReviewQueueItem(
  deploymentId: string,
  prediction?: ReviewQueuePredictionFilter,
) {
  const queryClient = useQueryClient();
  return useCallback(
    (traceId: string) => {
      queryClient.setQueryData<ReviewQueueInfiniteData>(
        evalKeys.reviewQueue(deploymentId, prediction),
        (old) => removeReviewQueueItem(old, traceId),
      );
    },
    [queryClient, deploymentId, prediction],
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

export function useUndoDatasetJudgment(
  deploymentId: string,
  prediction?: ReviewQueuePredictionFilter,
) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<
    DatasetJudgmentResponse,
    Error,
    DatasetUndoJudgmentVariables
  >({
    mutationFn: ({ traceId }) =>
      api.deleteDatasetJudgment(deploymentId, traceId),
    onSuccess: async (_data, variables) => {
      const restoredItem = variables.reviewQueueItem;
      if (restoredItem) {
        queryClient.setQueryData<ReviewQueueInfiniteData>(
          evalKeys.reviewQueue(deploymentId, prediction),
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

      // Queue-originated undo stays optimistic because it already has the full
      // item. Dataset-originated undo reloads the server-owned prediction data.
      await Promise.all(invalidations);
    },
  });
}

export function useSetDatasetJudgmentCriteria(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<
    DatasetJudgmentCriteriaResponse,
    Error,
    DatasetJudgmentCriteriaVariables
  >({
    mutationFn: ({ traceId, criteria }) =>
      api.putDatasetJudgmentCriteria(deploymentId, traceId, { criteria }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ]);
    },
  });
}
