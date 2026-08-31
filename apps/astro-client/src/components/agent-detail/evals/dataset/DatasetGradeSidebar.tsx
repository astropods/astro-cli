import { useState } from "react";
import { ChevronDown } from "lucide-react";
import { InlineBadge } from "@/components/InlineBadge";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import type {
  EvalDatasetEvaluatorSummary,
  EvalDatasetResponse,
  EvalDatasetValueCount,
} from "@/lib/api";
import { formatEvaluatorValue } from "../evaluator-values";

export function DatasetGradeSidebar({
  summary,
}: {
  summary: EvalDatasetResponse;
}) {
  return (
    <aside className="flex w-full flex-none flex-col gap-4 border-b border-border bg-card p-4 @[780px]/dataset-card:w-[300px] @[780px]/dataset-card:border-b-0 @[780px]/dataset-card:border-r @[780px]/dataset-card:p-5">
      <h3 className="text-heading-4 text-foreground">Dataset overview</h3>

      {summary.evaluators.length === 0 ? (
        <div className="rounded-md border border-dashed border-border px-4 py-5 text-body-sm text-muted-foreground">
          No evaluator values recorded yet.
        </div>
      ) : (
        <div className="flex flex-col gap-0.5">
          {summary.evaluators.map((evaluator) => (
            <EvaluatorBreakdown key={evaluator.key} evaluator={evaluator} />
          ))}
        </div>
      )}
    </aside>
  );
}

function EvaluatorBreakdown({
  evaluator,
}: {
  evaluator: EvalDatasetEvaluatorSummary;
}) {
  const [open, setOpen] = useState(false);
  const ranked = [...evaluator.distribution].sort((a, b) => b.count - a.count);

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex w-full items-center gap-2 rounded-md py-2 pr-1 text-left transition-colors hover:bg-muted/40">
        <ChevronDown
          aria-hidden
          className={cn(
            "size-4 flex-none text-muted-foreground transition-transform",
            !open && "-rotate-90",
          )}
        />
        <span className="min-w-0 flex-1 truncate text-body-sm font-medium text-foreground">
          {evaluator.label}
        </span>
        <InlineBadge
          variant="fill"
          shape="square"
          className="flex-none px-2 py-0.5 text-mono-xs tabular-nums"
        >
          {totalCount(evaluator.distribution)}
        </InlineBadge>
      </CollapsibleTrigger>
      <CollapsibleContent>
        {/* bg-accent, not muted: muted resolves to the card behind it in dark mode. */}
        <div className="mb-1 mr-1 flex flex-col gap-2.5 rounded-md bg-accent px-3 py-3">
          {ranked.map((entry) => (
            <div
              key={String(entry.value)}
              className="flex items-baseline justify-between gap-3 text-body-sm text-muted-foreground"
            >
              <span className="min-w-0 truncate">
                {formatEvaluatorValue(entry.value)}
              </span>
              <span className="flex-none font-mono text-mono-xs tabular-nums">
                {entry.count}
              </span>
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function totalCount(distribution: EvalDatasetValueCount[]) {
  return distribution.reduce((sum, entry) => sum + entry.count, 0);
}
