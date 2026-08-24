import type { ReactNode } from "react";
import { ChevronRight, Loader2, Sparkle } from "lucide-react";
import { cn } from "@/lib/utils";

export function ReviewQueueEvaluationControls({
  resultsOpen,
  onResultsOpenChange,
  loading = false,
  noResults = false,
  actions,
}: {
  resultsOpen: boolean;
  onResultsOpenChange: (open: boolean) => void;
  loading?: boolean;
  noResults?: boolean;
  actions?: ReactNode;
}) {
  const toggle = () => onResultsOpenChange(!resultsOpen);

  return (
    <div
      onClick={(event) => {
        const target = event.target;
        if (target instanceof Element && target.closest("button, a")) return;
        toggle();
      }}
      className="flex w-full cursor-pointer items-center gap-2"
    >
      <button
        type="button"
        onClick={toggle}
        aria-expanded={resultsOpen}
        aria-label={`${resultsOpen ? "Collapse" : "Expand"} evaluation results`}
        className="flex flex-none items-center py-2.5"
      >
        <ChevronRight
          aria-hidden
          className={cn(
            "size-3.5 text-muted-foreground transition-transform",
            resultsOpen && "rotate-90",
          )}
        />
      </button>
      {loading ? (
        <Loader2
          aria-hidden
          className="dp-spin size-4 flex-none text-muted-foreground"
        />
      ) : (
        <Sparkle
          aria-hidden
          className={cn(
            "size-4 flex-none",
            noResults ? "text-muted-foreground" : "text-primary",
          )}
        />
      )}
      <div className="flex min-w-0 flex-1 items-baseline gap-1.5 py-2.5 text-body font-semibold text-foreground">
        Evaluation results
        {noResults && (
          <span className="text-body-sm font-normal text-muted-foreground">
            No results
          </span>
        )}
      </div>
      {actions}
    </div>
  );
}
