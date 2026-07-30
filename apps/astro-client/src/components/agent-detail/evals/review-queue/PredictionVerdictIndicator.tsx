import { Sparkle } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import type { ProgressBarTone } from "@/components/ui/progress-bar";
import type {
  ReviewQueuePrediction,
  ReviewQueuePredictionStatus,
} from "@/lib/api";
import { cn } from "@/lib/utils";

export type PredictionVerdict = "good" | "bad" | "unknown";

const VERDICT_PRESENTATION: Record<
  PredictionVerdict,
  { label: string; textClassName: string; tone: ProgressBarTone }
> = {
  good: {
    label: "Good",
    textClassName: "text-success",
    tone: "success",
  },
  bad: {
    label: "Bad",
    textClassName: "text-destructive",
    tone: "destructive",
  },
  unknown: {
    label: "Not sure",
    textClassName: "text-muted-foreground",
    tone: "muted",
  },
};

export function predictionVerdictPresentation(verdict: PredictionVerdict) {
  return VERDICT_PRESENTATION[verdict];
}

const VERDICT_BADGE_COLOR = {
  good: "success",
  bad: "error",
  unknown: "muted",
} as const;

export function predictionVerdict(score: number): PredictionVerdict {
  if (score >= 0.25) return "good";
  if (score <= -0.25) return "bad";
  return "unknown";
}

export function PredictionVerdictBadge({
  verdict,
  className,
}: {
  verdict: PredictionVerdict;
  className?: string;
}) {
  const presentation = predictionVerdictPresentation(verdict);
  return (
    <StatusBadge
      color={VERDICT_BADGE_COLOR[verdict]}
      size="sm"
      className={cn("flex-none whitespace-nowrap !rounded-sm", className)}
    >
      <Sparkle aria-hidden className="size-3.5" />
      <span className="font-sans text-body-sm font-semibold tracking-normal">
        {presentation.label}
      </span>
    </StatusBadge>
  );
}

export function PredictionVerdictIndicator({
  prediction,
  status,
  className,
}: {
  prediction: ReviewQueuePrediction | null;
  status: ReviewQueuePredictionStatus;
  className?: string;
}) {
  if (
    !prediction &&
    (status === "queued" || status === "in_progress")
  ) {
    return (
      <StatusBadge
        color="primary"
        size="sm"
        indicator
        spinning
        className={cn(
          "flex-none whitespace-nowrap !border-transparent",
          className,
        )}
      >
        <span className="font-sans text-body-sm font-semibold tracking-normal">
          Judging…
        </span>
      </StatusBadge>
    );
  }

  if (!prediction) {
    return (
      <StatusBadge
        color="muted"
        outline
        className={cn("flex-none whitespace-nowrap border-dashed", className)}
      >
        <Sparkle aria-hidden className="size-3.5" />
        <span className="font-sans text-body-sm font-semibold tracking-normal">
          Not judged
        </span>
      </StatusBadge>
    );
  }

  const verdict = predictionVerdict(prediction.verdict_score);
  const presentation = predictionVerdictPresentation(verdict);

  return (
    <div
      className={cn(
        "flex w-24 flex-none items-center justify-end gap-1.5 font-sans text-body-sm font-semibold tracking-normal",
        presentation.textClassName,
        className,
      )}
    >
      <Sparkle aria-hidden className="size-3.5" />
      {presentation.label}
    </div>
  );
}
