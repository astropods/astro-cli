import { Loader2, Sparkle } from "lucide-react";
import { isRunActive } from "@/api/queries/evals";
import type { EvaluationRun } from "@/lib/api";
import { cn } from "@/lib/utils";

export function EvaluationStatusIndicator({
  run,
  className,
}: {
  run: EvaluationRun | null;
  className?: string;
}) {
  if (isRunActive(run)) {
    return (
      <Loader2
        aria-label="Evaluating"
        className={cn(
          "dp-spin size-4 flex-none text-muted-foreground",
          className,
        )}
      />
    );
  }

  const failed = run?.status === "failed";
  if (!failed && run?.status !== "completed") {
    return null;
  }

  return (
    <Sparkle
      aria-label={failed ? "Couldn’t evaluate" : "Evaluated"}
      className={cn(
        "size-4 flex-none",
        failed ? "text-muted-foreground" : "text-primary",
        className,
      )}
    />
  );
}
