import { useCallback, useMemo, useState, type RefObject } from "react";
import { toast } from "sonner";
import { Spinner } from "@/components/ui/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableShowMore,
} from "@/components/ui/table";
import { useAccountMembers } from "@/api/queries/accounts";
import type {
  EvalDatasetItem,
  EvalDatasetResponse,
  EvaluationSetEvaluator,
  EvaluatorOutputValue,
} from "@/lib/api";
import {
  useEvalDatasetItems,
  useRemoveDatasetItem,
  useSetDatasetItemOutputs,
} from "@/api/queries/evals";

import {
  DATASET_ITEM_COLUMN_COUNT,
  DatasetItemRow,
  type ResolvedReviewer,
} from "./DatasetItemRow";
import { flyUndoToReviewQueue } from "../review-queue-motion";

interface DatasetTableProps {
  deploymentId: string;
  account: string;
  summary: EvalDatasetResponse;
  evaluators: EvaluationSetEvaluator[];
  evaluationRef?: string;
  evaluatorsUnavailable: boolean;
  reviewQueueTargetRef?: RefObject<HTMLElement | null>;
}

const PAGE_LIMIT = 50;

export function DatasetTable({
  deploymentId,
  account,
  summary,
  evaluators,
  evaluationRef,
  evaluatorsUnavailable,
  reviewQueueTargetRef,
}: DatasetTableProps) {
  const { data, isLoading, isError, fetchNextPage, hasNextPage } =
    useEvalDatasetItems(deploymentId, PAGE_LIMIT);
  const removeItem = useRemoveDatasetItem(deploymentId);
  const setOutputs = useSetDatasetItemOutputs(deploymentId);
  const removingTraceId = removeItem.isPending
    ? removeItem.variables?.traceId ?? null
    : null;
  const savingOutputsTraceId = setOutputs.isPending
    ? setOutputs.variables?.traceId ?? null
    : null;

  const [expandedId, setExpandedId] = useState<string | null>(null);
  const toggleExpanded = useCallback((id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  }, []);
  const removeTrace = useCallback(
    (item: EvalDatasetItem, trigger: HTMLElement | null) => {
      const sourceRect = trigger?.getBoundingClientRect() ?? null;
      removeItem.reset();
      removeItem.mutate(
        {
          traceId: item.source_trace_id,
        },
        {
          onSuccess: () =>
            flyUndoToReviewQueue(sourceRect, reviewQueueTargetRef?.current),
        },
      );
    },
    [reviewQueueTargetRef, removeItem],
  );
  const saveTraceOutputs = useCallback(
    (traceId: string, outputs: EvaluatorOutputValue[], onSaved: () => void) => {
      setOutputs.mutate(
        { traceId, outputs },
        {
          onSuccess: () => {
            toast.success("Evaluator values saved");
            onSaved();
          },
          onError: () =>
            toast.error("Could not save evaluator values. Try again."),
        },
      );
    },
    [setOutputs],
  );

  const { data: membersData, isLoading: membersLoading } = useAccountMembers(
    account,
    {
      enabled: !!account,
    },
  );
  const memberById = useMemo(() => {
    const m = new Map<string, { username: string; display_name?: string }>();
    for (const member of membersData?.members ?? []) {
      m.set(member.user_id, member);
    }
    return m;
  }, [membersData]);

  const resolveReviewer = useCallback(
    (userId: string | undefined): ResolvedReviewer | null => {
      if (!userId) return null;
      const member = memberById.get(userId);
      if (member) {
        return {
          handle: member.username,
          name: member.display_name || member.username,
        };
      }
      return { name: membersLoading ? "…" : "Unknown user" };
    },
    [memberById, membersLoading],
  );

  const allItems = useMemo<EvalDatasetItem[]>(
    () => data?.pages.flatMap((p) => p.items) ?? [],
    [data?.pages],
  );

  const emptyMessage =
    summary.item_count === 0
      ? "No items yet."
      : "No items loaded.";

  return (
    <Table
      bare
      className="block w-full @[760px]/dataset-table:table @[760px]/dataset-table:table-fixed"
      containerClassName="@container/dataset-table flex min-w-0 flex-1 flex-col bg-background"
      footer={
        hasNextPage && !isLoading ? (
          <TableShowMore
            hiddenCount={Math.max(
              0,
              Math.min(
                PAGE_LIMIT,
                (data?.pages[0]?.total_items ?? summary.item_count) -
                  allItems.length,
              ),
            )}
            expanded={false}
            onToggle={() => void fetchNextPage()}
          />
        ) : null
      }
    >
      <TableHeader className="hidden bg-black/2 dark:bg-white/3 @[760px]/dataset-table:table-header-group">
        <TableRow>
          <TableHead className="w-10 pl-5 pr-0 text-faint-foreground" />
          <TableHead className="w-[34%] text-faint-foreground">Input</TableHead>
          <TableHead className="w-[42%] text-faint-foreground">
            Expected output
          </TableHead>
          <TableHead className="w-[170px] text-faint-foreground">
            Evaluators
          </TableHead>
          <TableHead className="w-[185px] pr-5 text-faint-foreground">
            Verified by
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody className="block @[760px]/dataset-table:table-row-group">
        {removeItem.isError && (
          <TableRow>
            <TableCell
              colSpan={DATASET_ITEM_COLUMN_COUNT}
              className="py-3 text-center text-body-sm text-muted-foreground"
            >
              Could not update trace. Try again.
            </TableCell>
          </TableRow>
        )}
        {isLoading && (
          <TableRow>
            <TableCell
              colSpan={DATASET_ITEM_COLUMN_COUNT}
              className="py-10 text-center"
            >
              <Spinner delay={300} />
            </TableCell>
          </TableRow>
        )}
        {isError && !isLoading && (
          <TableRow>
            <TableCell
              colSpan={DATASET_ITEM_COLUMN_COUNT}
              className="py-12 text-center text-body-sm text-muted-foreground"
            >
              Failed to load items.
            </TableCell>
          </TableRow>
        )}
        {!isLoading && !isError && allItems.length === 0 && (
          <TableRow>
            <TableCell
              colSpan={DATASET_ITEM_COLUMN_COUNT}
              className="py-12 text-center text-body-sm text-muted-foreground"
            >
              {emptyMessage}
            </TableCell>
          </TableRow>
        )}
        {allItems.map((item) => (
          <DatasetItemRow
            key={item.id}
            item={item}
            evaluators={evaluators}
            evaluationRef={evaluationRef}
            evaluatorsUnavailable={evaluatorsUnavailable}
            isOpen={expandedId === item.id}
            onToggle={toggleExpanded}
            onRemove={(trigger) => removeTrace(item, trigger)}
            onSaveOutputs={saveTraceOutputs}
            isRemoving={removingTraceId === item.source_trace_id}
            isSavingOutputs={savingOutputsTraceId === item.source_trace_id}
            reviewer={resolveReviewer(item.verified_by_user_id)}
          />
        ))}
      </TableBody>
    </Table>
  );
}
