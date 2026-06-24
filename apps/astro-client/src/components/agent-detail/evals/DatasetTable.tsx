import { useCallback, useMemo, useState } from "react";
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
  EvalDatasetItemsVerdict,
  EvalDatasetResponse,
} from "@/lib/api";
import { useEvalDatasetItems } from "@/api/queries/evals";

import type { FilterKey } from "./DatasetFilterChips";
import {
  DATASET_ITEM_COLUMN_COUNT,
  DatasetItemRow,
  type RawMode,
  type ResolvedReviewer,
} from "./DatasetItemRow";

export interface DatasetTableProps {
  deploymentId: string;
  account: string;
  summary: EvalDatasetResponse;
  selected: Set<FilterKey>;
  rawMode: RawMode;
}

const PAGE_LIMIT = 50;

export function DatasetTable({
  deploymentId,
  account,
  summary,
  selected,
  rawMode,
}: DatasetTableProps) {
  const activeVerdict: EvalDatasetItemsVerdict | undefined =
    selected.size === 1 ? (selected.has("good") ? "good" : "bad") : undefined;

  const { data, isLoading, isError, fetchNextPage, hasNextPage } =
    useEvalDatasetItems(deploymentId, PAGE_LIMIT, activeVerdict);

  const [expandedId, setExpandedId] = useState<string | null>(null);
  const toggleExpanded = useCallback((id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  }, []);

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
      containerClassName="flex min-w-0 flex-1 flex-col bg-background"
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
      <TableHeader className="bg-black/2 dark:bg-white/3">
        <TableRow>
          <TableHead className="w-4 pl-5 pr-0 text-faint-foreground" />
          <TableHead className="w-[78px] text-faint-foreground">
            Verdict
          </TableHead>
          <TableHead className="text-faint-foreground">Input</TableHead>
          <TableHead className="text-faint-foreground">
            Expected output
          </TableHead>
          <TableHead className="w-[178px] pr-5 text-faint-foreground">
            Judged by
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
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
            reviewer={resolveReviewer(item.metadata?.judged_by_user_id)}
            rawMode={rawMode}
          />
        ))}
      </TableBody>
    </Table>
  );
}
