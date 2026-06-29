import {
  type InfiniteData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useApiClient } from "../../lib/api-context";
import type {
  DatasetJudgmentResponse,
  DatasetJudgmentVerdict,
  EvalDatasetItemsResponse,
  EvalDatasetItemsVerdict,
  EvalDatasetResponse,
  ReviewQueueItem,
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
};

type DatasetUndoJudgmentVariables = {
  traceId: string;
  reviewQueueItem?: ReviewQueueItem;
  reviewQueuePageIndex?: number;
};

type DatasetChangeJudgmentVariables = {
  traceId: string;
  verdict: DatasetJudgmentVerdict;
};

type ReviewQueuePageParam =
  | {
      offset: number;
      endTime: string;
    }
  | undefined;

type ReviewQueueInfiniteData = InfiniteData<
  ReviewQueueResponse,
  ReviewQueuePageParam
>;

const REVIEW_QUEUE_PAGE_SIZE = 50;

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
  verdict?: EvalDatasetItemsVerdict,
  enabled = true,
) {
  const api = useApiClient();
  return useInfiniteQuery({
    queryKey: evalKeys.items(deploymentId, limit, verdict),
    queryFn: ({
      pageParam,
    }: {
      pageParam: number | string | undefined;
    }): Promise<EvalDatasetItemsResponse> =>
      api.getEvalDatasetItems(deploymentId, {
        page: verdict ? undefined : typeof pageParam === "number" ? pageParam : 1,
        cursor: verdict && typeof pageParam === "string" ? pageParam : undefined,
        limit,
        verdict,
      }),
    initialPageParam: undefined as number | string | undefined,
    getNextPageParam: (last) =>
      verdict
        ? last.next_cursor || undefined
        : last.page < last.total_pages
          ? last.page + 1
          : undefined,
    enabled: !!deploymentId && enabled,
    staleTime: 60_000,
  });
}

export function useDatasetReviewQueue(deploymentId: string, enabled = true) {
  const api = useApiClient();
  return useInfiniteQuery({
    queryKey: evalKeys.reviewQueue(deploymentId),
    queryFn: ({
      pageParam,
    }: {
      pageParam: ReviewQueuePageParam;
    }): Promise<ReviewQueueResponse> =>
      api.getDatasetReviewQueue(deploymentId, {
        limit: REVIEW_QUEUE_PAGE_SIZE,
        offset: pageParam?.offset,
        endTime: pageParam?.endTime,
      }),
    initialPageParam: undefined as ReviewQueuePageParam,
    getNextPageParam: (last, allPages): ReviewQueuePageParam => {
      if (last.next_offset == null || last.next_offset <= 0) {
        return undefined;
      }

      const snapshotEndTime = allPages[0]?.end_time;
      return snapshotEndTime
        ? { offset: last.next_offset, endTime: snapshotEndTime }
        : undefined;
    },
    enabled: !!deploymentId && enabled,
    staleTime: 30_000,
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

      queryClient.setQueryData<ReviewQueueInfiniteData>(
        evalKeys.reviewQueue(deploymentId),
        (old) => removeReviewQueueItem(old, variables.traceId),
      );

      // Skip the review-queue here — the optimistic update already reflects
      // the removal, and refetching forces an expensive Langfuse round trip
      // plus sentiment annotation per click. The query's staleTime picks
      // up new traces on the next remount.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ]);
    },
  });
}

function insertReviewQueueItem(
  items: ReviewQueueItem[],
  restored: ReviewQueueItem,
) {
  return [restored, ...items.filter((item) => item.trace_id !== restored.trace_id)]
    .sort((a, b) => {
      const aHasSentiment = a.sentiment !== "";
      const bHasSentiment = b.sentiment !== "";
      if (aHasSentiment !== bHasSentiment) {
        return aHasSentiment ? -1 : 1;
      }
      return b.timestamp.localeCompare(a.timestamp);
    });
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

export function useUndoDatasetJudgment(deploymentId: string) {
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
          evalKeys.reviewQueue(deploymentId),
          (old) =>
            insertReviewQueueItemPage(
              old,
              restoredItem,
              variables.reviewQueuePageIndex,
            ),
        );
      }

      // Keep the restored queue item optimistic, matching the judge path.
      // Refetching an infinite review queue would replay every loaded page's
      // expensive Langfuse fetch and sentiment annotation for a single undo.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ]);
    },
  });
}

export function useChangeDatasetJudgment(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();

  return useMutation<
    DatasetJudgmentResponse,
    Error,
    DatasetChangeJudgmentVariables
  >({
    mutationFn: ({ traceId, verdict }) =>
      api.patchDatasetJudgment(deploymentId, traceId, { verdict }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
      ]);
    },
  });
}
