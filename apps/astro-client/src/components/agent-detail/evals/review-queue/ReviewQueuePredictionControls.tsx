import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ReviewQueuePrediction } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  PredictionVerdictBadge,
  predictionVerdict,
} from "./PredictionVerdictIndicator";

export function ReviewQueuePredictionControls({
  prediction,
  explanationOpen,
  onExplanationOpenChange,
}: {
  prediction: ReviewQueuePrediction;
  explanationOpen: boolean;
  onExplanationOpenChange: (open: boolean) => void;
}) {
  const predictedVerdict = predictionVerdict(prediction.verdict_score);

  return (
    <div className="flex min-w-0 flex-nowrap items-center gap-2">
      <PredictionVerdictBadge verdict={predictedVerdict} />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-expanded={explanationOpen}
        onClick={() => onExplanationOpenChange(!explanationOpen)}
        className="shrink-0 whitespace-nowrap px-2 text-body-sm font-medium"
      >
        {explanationOpen ? "Hide" : "See"} explanation
        <ChevronDown
          aria-hidden
          className={cn(
            "size-4 text-muted-foreground transition-transform",
            explanationOpen && "rotate-180",
          )}
        />
      </Button>
    </div>
  );
}
