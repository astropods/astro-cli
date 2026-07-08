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
  DatasetJudgmentVerdict,
  EvalDatasetItem,
  EvalDatasetItemsVerdict,
  EvalDatasetResponse,
  JudgmentCriterion,
  ReviewQueueItem,
} from "@/lib/api";
import {
  useChangeDatasetJudgment,
  useEvalDatasetItems,
  useSetDatasetJudgmentCriteria,
  useUndoDatasetJudgment,
} from "@/api/queries/evals";

import type { FilterKey } from "./DatasetFilterChips";
import {
  DATASET_ITEM_COLUMN_COUNT,
  DatasetItemRow,
  type ResolvedReviewer,
} from "./DatasetItemRow";
import { flyUndoToReviewQueue } from "../review-queue-motion";

export interface DatasetTableProps {
  deploymentId: string;
  account: string;
  summary: EvalDatasetResponse;
  selected: Set<FilterKey>;
  reviewQueueTargetRef?: RefObject<HTMLElement | null>;
}

const PAGE_LIMIT = 50;

export function DatasetTable({
  deploymentId,
  account,
  summary,
  selected,
  reviewQueueTargetRef,
}: DatasetTableProps) {
  const activeVerdict: EvalDatasetItemsVerdict | undefined =
    selected.size === 1 ? (selected.has("good") ? "good" : "bad") : undefined;

  const { data, isLoading, isError, fetchNextPage, hasNextPage } =
    useEvalDatasetItems(deploymentId, PAGE_LIMIT, activeVerdict);
  const undoJudgment = useUndoDatasetJudgment(deploymentId);
  const changeJudgment = useChangeDatasetJudgment(deploymentId);
  const setCriteria = useSetDatasetJudgmentCriteria(deploymentId);
  const undoingTraceId = undoJudgment.isPending
    ? undoJudgment.variables?.traceId ?? null
    : null;
  const changingTraceId = changeJudgment.isPending
    ? changeJudgment.variables?.traceId ?? null
    : null;
  const savingCriteriaTraceId = setCriteria.isPending
    ? setCriteria.variables?.traceId ?? null
    : null;

  const [expandedId, setExpandedId] = useState<string | null>(null);
  const toggleExpanded = useCallback((id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  }, []);
  const undoTrace = useCallback(
    (item: EvalDatasetItem, trigger: HTMLElement | null) => {
      const sourceRect = trigger?.getBoundingClientRect() ?? null;
      undoJudgment.reset();
      undoJudgment.mutate(
        {
          traceId: item.source_trace_id,
          reviewQueueItem: datasetItemToReviewQueueItem(item),
        },
        {
          onSuccess: () =>
            flyUndoToReviewQueue(sourceRect, reviewQueueTargetRef?.current),
        },
      );
    },
    [reviewQueueTargetRef, undoJudgment],
  );
  const changeTraceVerdict = useCallback(
    (traceId: string, verdict: DatasetJudgmentVerdict) => {
      changeJudgment.reset();
      changeJudgment.mutate({ traceId, verdict });
    },
    [changeJudgment],
  );
  const saveTraceCriteria = useCallback(
    (traceId: string, criteria: JudgmentCriterion[], onSaved: () => void) => {
      setCriteria.mutate(
        { traceId, criteria },
        {
          onSuccess: () => {
            toast.success("Criteria saved");
            onSaved();
          },
          onError: () => toast.error("Could not save criteria. Try again."),
        },
      );
    },
    [setCriteria],
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
      : activeVerdict
        ? "No matching items."
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
          <TableHead className="w-[92px] text-faint-foreground">
            Verdict
          </TableHead>
          <TableHead className="w-[30%] text-faint-foreground">Input</TableHead>
          <TableHead className="w-[38%] text-faint-foreground">
            Expected output
          </TableHead>
          <TableHead className="w-[170px] text-faint-foreground">Reason</TableHead>
          <TableHead className="w-[185px] pr-5 text-faint-foreground">
            Judged by
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody className="block @[760px]/dataset-table:table-row-group">
        {(undoJudgment.isError || changeJudgment.isError) && (
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
            isOpen={expandedId === item.id}
            onToggle={toggleExpanded}
            onChangeVerdict={changeTraceVerdict}
            onRemoveVerdict={(_traceId, trigger) => undoTrace(item, trigger)}
            onSaveCriteria={saveTraceCriteria}
            isChanging={changingTraceId === item.source_trace_id}
            isRemoving={undoingTraceId === item.source_trace_id}
            isSavingCriteria={savingCriteriaTraceId === item.source_trace_id}
            reviewer={resolveReviewer(item.metadata?.judged_by_user_id)}
          />
        ))}
      </TableBody>
    </Table>
  );
}

function datasetItemToReviewQueueItem(item: EvalDatasetItem): ReviewQueueItem {
  return {
    trace_id: item.source_trace_id,
    timestamp: item.created_at,
    input: item.input,
    output: item.expected_output,
    sentiment: "",
  };
}
