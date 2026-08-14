import { ChevronRight, Info } from "lucide-react";
import { ContentValue } from "@/components/agent-detail/ContentValue";
import { CriterionLabels } from "@/components/agent-detail/evals/CriterionLabels";
import { UserAvatar } from "@/components/UserAvatar";
import { TableCell, TableRow } from "@/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { summarize } from "@/lib/content-parse";
import { formatTimeAgo } from "@/lib/time-format";
import type { EvalDatasetItem, JudgmentCriterion } from "@/lib/api";
import { DatasetRowActionsMenu } from "./DatasetRowActionsMenu";

export const DATASET_ITEM_COLUMN_COUNT = 5;

export interface ResolvedReviewer {
  handle?: string;
  name: string;
}

function ReviewerCell({
  reviewer,
  judgedAt,
}: {
  reviewer: ResolvedReviewer | null;
  judgedAt?: string;
}) {
  if (!reviewer) return <span className="text-faint-foreground">—</span>;
  const { handle, name } = reviewer;
  const ago = judgedAt ? formatTimeAgo(judgedAt) : "";

  return (
    <div className="flex min-w-0 items-center gap-2.5" title={name}>
      {handle ? (
        <UserAvatar handle={handle} name={name} className="size-7 flex-none" />
      ) : (
        <span
          aria-hidden
          className="flex size-7 flex-none items-center justify-center rounded-full border border-border bg-muted text-body-sm font-medium text-muted-foreground"
        >
          {name.slice(0, 1).toUpperCase()}
        </span>
      )}
      <div className="flex min-w-0 flex-col gap-px">
        <span className="truncate text-body-sm text-foreground">{name}</span>
        {ago && (
          <span className="truncate font-mono text-mono-sm text-muted-foreground">{ago}</span>
        )}
      </div>
    </div>
  );
}

function ExpandedPreview({
  input,
  output,
}: {
  input: unknown;
  output: unknown;
}) {
  return (
    <div className="mx-0 animate-in fade-in slide-in-from-top-1 border-t border-dashed border-border px-4 py-4 duration-150 ease-out @[760px]/dataset-table:ml-8 @[760px]/dataset-table:mr-3 @[760px]/dataset-table:px-5">
      <div className="flex flex-col gap-5 @[760px]/dataset-table:flex-row @[760px]/dataset-table:items-start @[760px]/dataset-table:gap-7">
        <div className="min-w-0 flex-1">
          <div className="mb-2 font-mono text-label uppercase text-faint-foreground">Input</div>
          <ContentValue content={input} mode="pretty" tone="foreground" />
        </div>
        <div className="hidden w-px flex-none self-stretch bg-border @[760px]/dataset-table:block" />
        <div className="min-w-0 flex-1">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <span className="font-mono text-label uppercase text-faint-foreground">
              Expected output
            </span>
            <TooltipProvider delayDuration={300}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="inline-flex cursor-help text-faint-foreground">
                    <Info aria-hidden className="size-3.5" />
                  </span>
                </TooltipTrigger>
                <TooltipContent className="max-w-xs">
                  The agent output captured with this response, kept as the reference for
                  evaluations. Future agent responses are compared with this example.
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <ContentValue content={output} mode="pretty" tone="muted" />
        </div>
      </div>
    </div>
  );
}

export interface DatasetItemRowProps {
  item: EvalDatasetItem;
  isOpen: boolean;
  onToggle: (id: string) => void;
  onRemove: (trigger: HTMLElement | null) => void;
  onSaveCriteria: (
    traceId: string,
    criteria: JudgmentCriterion[],
    onSaved: () => void,
  ) => void;
  isRemoving: boolean;
  isSavingCriteria: boolean;
  reviewer: ResolvedReviewer | null;
}

export function DatasetItemRow({
  item,
  isOpen,
  onToggle,
  onRemove,
  onSaveCriteria,
  isRemoving,
  isSavingCriteria,
  reviewer,
}: DatasetItemRowProps) {
  const savedCriteriaKeys =
    item.metadata?.judgment_criteria
      ?.filter((criterion) => criterion.value > 0)
      .map((criterion) => criterion.dimension_key) ?? [];
  const inputSummary = summarize(item.input);
  const outputSummary = summarize(item.expected_output);

  const toggle = () => onToggle(item.id);
  const onKeyDown = (e: React.KeyboardEvent<HTMLTableRowElement>) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      toggle();
    }
  };

  return (
    <>
      <TableRow
        onClick={toggle}
        onKeyDown={onKeyDown}
        role="button"
        tabIndex={0}
        aria-expanded={isOpen}
        className={cn(
          "relative flex cursor-pointer flex-col align-top transition-colors hover:bg-muted/40 @[760px]/dataset-table:table-row",
          isOpen && "border-b-0",
        )}
      >
        <TableCell
          className={cn(
            "absolute left-4 top-4 block p-0 @[760px]/dataset-table:static @[760px]/dataset-table:table-cell @[760px]/dataset-table:py-3.5 @[760px]/dataset-table:pl-5 @[760px]/dataset-table:pr-0",
            isOpen && "shadow-[inset_3px_0_0_var(--color-primary)]",
          )}
        >
          <ChevronRight
            aria-hidden
            className={cn(
              "size-3 text-muted-foreground transition-transform",
              isOpen && "rotate-90",
            )}
          />
        </TableCell>
        <TableCell
          data-label="Input"
          className="order-3 block px-4 py-2 before:mb-1 before:block before:font-mono before:text-label before:uppercase before:text-faint-foreground before:content-[attr(data-label)] @[760px]/dataset-table:table-cell @[760px]/dataset-table:py-3.5 @[760px]/dataset-table:before:hidden"
        >
          <div className="line-clamp-2 text-body text-foreground" title={inputSummary}>
            {inputSummary}
          </div>
        </TableCell>
        <TableCell
          data-label="Expected output"
          className="order-4 block px-4 pb-4 pt-2 before:mb-1 before:block before:font-mono before:text-label before:uppercase before:text-faint-foreground before:content-[attr(data-label)] @[760px]/dataset-table:table-cell @[760px]/dataset-table:py-3.5 @[760px]/dataset-table:before:hidden"
        >
          <div
            className="line-clamp-2 text-body text-muted-foreground"
            title={outputSummary}
          >
            {outputSummary}
          </div>
        </TableCell>
        <TableCell
          data-label="Criteria"
          className="order-5 block px-4 pb-2 pt-2 before:mb-1 before:block before:font-mono before:text-label before:uppercase before:text-faint-foreground before:content-[attr(data-label)] @[760px]/dataset-table:table-cell @[760px]/dataset-table:py-3.5 @[760px]/dataset-table:before:hidden"
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          <CriterionLabels criteria={item.metadata?.judgment_criteria ?? []} />
        </TableCell>
        <TableCell className="order-2 block px-4 pb-2 pt-1 @[760px]/dataset-table:table-cell @[760px]/dataset-table:py-3.5 @[760px]/dataset-table:pr-5">
          <div className="flex min-w-0 items-center justify-between gap-3">
            <ReviewerCell reviewer={reviewer} judgedAt={item.metadata?.judged_at} />
            <div
              className="flex flex-none"
              onClick={(event) => event.stopPropagation()}
              onKeyDown={(event) => event.stopPropagation()}
            >
              <DatasetRowActionsMenu
                traceId={item.source_trace_id}
                savedCriteriaKeys={savedCriteriaKeys}
                isRemoving={isRemoving}
                onRemove={onRemove}
                onSaveCriteria={onSaveCriteria}
                isSavingCriteria={isSavingCriteria}
              />
            </div>
          </div>
        </TableCell>
      </TableRow>
      {isOpen && (
        <TableRow className="block @[760px]/dataset-table:table-row">
          <TableCell
            colSpan={DATASET_ITEM_COLUMN_COUNT}
            className="block p-0 shadow-[inset_3px_0_0_var(--color-primary)] @[760px]/dataset-table:table-cell"
          >
            <ExpandedPreview
              input={item.input}
              output={item.expected_output}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}
