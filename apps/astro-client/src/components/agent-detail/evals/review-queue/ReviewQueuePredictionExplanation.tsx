import { Check, CircleAlert, Sparkle } from "lucide-react";
import { InfoHint } from "@/components/InfoHint";
import { StatusBadge, type StatusBadgeColor } from "@/components/StatusBadge";
import { ProgressBar } from "@/components/ui/progress-bar";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { ReviewQueuePrediction } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  JUDGMENT_CRITERIA,
  predictionCriterionAssessment,
  predictionCriterionValue,
  type PredictionCriterionAssessment,
} from "../judgment-criteria";
import {
  predictionVerdict,
  predictionVerdictPresentation,
} from "./PredictionVerdictIndicator";

function criterionPresentation(
  assessment: PredictionCriterionAssessment,
): {
  color: StatusBadgeColor;
  Icon: typeof Check;
} {
  if (assessment === "accepted") return { color: "success", Icon: Check };
  if (assessment === "rejected") {
    return { color: "error", Icon: CircleAlert };
  }
  return { color: "warning", Icon: CircleAlert };
}

export function ReviewQueuePredictionExplanation({
  prediction,
}: {
  prediction: ReviewQueuePrediction;
}) {
  const verdict = predictionVerdict(prediction.verdict_score);
  const presentation = predictionVerdictPresentation(verdict);
  const confidence = Math.min(100, Math.max(0, prediction.confidence));

  return (
    <div className="pt-4 pb-2">
      <div className="flex items-center gap-2 font-mono text-mono-sm text-faint-foreground">
        <Sparkle aria-hidden className="size-4 text-primary" />
        Judge&apos;s verdict
      </div>
      <div className="mt-3">
        <div className="flex flex-wrap items-baseline justify-between gap-4">
          <div
            className={cn(
              "flex-none text-heading-2 font-semibold",
              presentation.textClassName,
            )}
          >
            {presentation.label}
          </div>
          <div className="flex flex-none items-center gap-1.5 text-body">
            <span className="font-semibold text-foreground tabular-nums">
              {confidence}% confident
            </span>
            <TooltipProvider delayDuration={300}>
              <InfoHint label="About judge confidence">
                How certain the judge is of its verdict, independent of which
                way it leans. Low confidence is worth a closer look.
              </InfoHint>
            </TooltipProvider>
          </div>
        </div>
        <ProgressBar
          aria-label="Judge confidence"
          value={confidence}
          tone={presentation.tone}
          className="mt-3"
        />
      </div>

      <p className="mt-6 max-w-5xl text-body leading-relaxed text-foreground">
        {prediction.explanation}
      </p>

      <div className="mt-3 flex flex-wrap gap-2">
        {JUDGMENT_CRITERIA.map(({ dimensionKey, dimensionLabel }) => {
          const assessment = predictionCriterionAssessment(
            predictionCriterionValue(prediction.criteria, dimensionKey),
          );
          const { color, Icon } = criterionPresentation(assessment);
          return (
            <StatusBadge
              key={dimensionKey}
              color={color}
              size="sm"
              className="!rounded-md"
            >
              <Icon aria-hidden className="size-3.5" />
              {dimensionLabel}
            </StatusBadge>
          );
        })}
      </div>
    </div>
  );
}
