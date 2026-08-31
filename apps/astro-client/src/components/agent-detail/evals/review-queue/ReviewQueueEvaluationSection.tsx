import { useMemo } from "react";
import { ChevronRight, Loader2, Sparkle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { InfoPanel } from "@/components/ui/status-panel";
import type {
  EvaluationSetEvaluator,
  EvaluatorOutputValue,
  TraceEvaluatorResult,
} from "@/lib/api";
import { useEvaluatorOutputSelection } from "../useEvaluatorOutputSelection";
import { completedOutputs, evaluationRows } from "./evaluation-rows";
import { ReviewQueueEvaluationResults } from "./ReviewQueueEvaluationResults";

interface ReviewQueueEvaluationSectionProps {
  evaluators: EvaluationSetEvaluator[];
  results: TraceEvaluatorResult[];
  scored: boolean;
  attempted?: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  loading?: boolean;
  isSaving: boolean;
  addError?: string;
  onAdd: (outputs: EvaluatorOutputValue[], trigger: HTMLElement | null) => void;
}

export function ReviewQueueEvaluationSection({
  evaluators,
  results,
  scored,
  attempted = false,
  open,
  onOpenChange,
  loading = false,
  isSaving,
  addError,
  onAdd,
}: ReviewQueueEvaluationSectionProps) {
  const rows = useMemo(
    () => evaluationRows(evaluators, results),
    [evaluators, results],
  );
  const initialOutputs = useMemo(() => completedOutputs(results), [results]);
  const { values, setValue, outputs, editedKeys } = useEvaluatorOutputSelection(
    rows,
    initialOutputs,
  );
  const awaitingFirstResult = loading && !scored;
  const noResults = attempted && !scored && !loading;

  return (
    <>
      <div
        data-review-queue-controls
        onClick={(event) => {
          const target = event.target;
          if (target instanceof Element && target.closest("button, a")) return;
          onOpenChange(!open);
        }}
        className="flex flex-none cursor-pointer items-center gap-2 border-b border-border bg-card px-4 py-3 dark:bg-surface @[520px]/review-card:px-6"
      >
        {loading ? (
          <Loader2
            aria-hidden
            className="dp-spin size-4 flex-none text-muted-foreground"
          />
        ) : (
          (scored || noResults) && (
            <Sparkle
              aria-hidden
              className={cn(
                "size-4 flex-none",
                scored ? "text-primary" : "text-foreground",
              )}
            />
          )
        )}
        <div className="flex min-w-0 flex-1 items-baseline gap-1.5 text-body font-semibold text-foreground">
          {scored || noResults ? "Evaluation results" : "Evaluate trace"}
          {noResults && (
            <span className="text-body-sm font-normal text-muted-foreground">
              No results
            </span>
          )}
        </div>
        <Button
          type="button"
          size="sm"
          aria-expanded={open}
          disabled={isSaving}
          onClick={() => onOpenChange(!open)}
        >
          <ChevronRight
            aria-hidden
            className={cn("size-3.5 transition-transform", open && "rotate-90")}
          />
          Add to dataset
        </Button>
      </div>

      {open && (
        <div className="flex flex-none flex-col gap-3 border-b border-border bg-card px-4 pb-3 dark:bg-surface @[520px]/review-card:px-6">
          {noResults && (
            <div className="mt-3">
              <InfoPanel variant="inline" size="xs">
                The evaluator couldn’t score this trace. Label it manually
                below.
              </InfoPanel>
            </div>
          )}

          <ReviewQueueEvaluationResults
            rows={rows}
            values={values}
            editedKeys={editedKeys}
            scored={scored}
            disabled={isSaving}
            loading={loading}
            onChange={setValue}
          />

          <div className="flex flex-wrap items-center justify-end gap-2">
            {addError && (
              <span className="mr-auto text-body-sm text-destructive">
                {addError}
              </span>
            )}
            <Button
              type="button"
              size="sm"
              disabled={isSaving || awaitingFirstResult}
              onClick={(event) => onAdd(outputs, event.currentTarget)}
            >
              {isSaving ? "Saving..." : "Save"}
            </Button>
          </div>
        </div>
      )}
    </>
  );
}
