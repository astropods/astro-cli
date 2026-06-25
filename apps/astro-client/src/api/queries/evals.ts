import {
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
};

type DatasetUndoJudgmentVariables = {
  traceId: string;
  reviewQueueItem?: ReviewQueueItem;
};

type DatasetChangeJudgmentVariables = {
  traceId: string;
  verdict: DatasetJudgmentVerdict;
};

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
  return useQuery({
    queryKey: evalKeys.reviewQueue(deploymentId),
    // TODO(eval-queue-pagination): keep the preview queue to one backend page
    // while we validate trace flow in preview. Follow-up before wider rollout:
    // convert this to useInfiniteQuery, pass next_offset with the original
    // end_time, and let ReviewQueueView fetch more rows when the local list
    // runs low.
    queryFn: (): Promise<ReviewQueueResponse> =>
      api.getDatasetReviewQueue(deploymentId),
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

      queryClient.setQueryData<ReviewQueueResponse>(
        evalKeys.reviewQueue(deploymentId),
        (old) =>
          old
            ? {
                ...old,
                items: old.items.filter(
                  (item) => item.trace_id !== variables.traceId,
                ),
              }
            : old,
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
        queryClient.setQueryData<ReviewQueueResponse>(
          evalKeys.reviewQueue(deploymentId),
          (old) =>
            old
              ? {
                  ...old,
                  items: insertReviewQueueItem(old.items, restoredItem),
                }
              : old,
        );
      }

      await Promise.all([
        queryClient.invalidateQueries({ queryKey: evalKeys.summary(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.itemsAll(deploymentId) }),
        queryClient.invalidateQueries({ queryKey: evalKeys.reviewQueue(deploymentId) }),
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
