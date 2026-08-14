import { Sparkle, TriangleAlert } from "lucide-react";
import { StatusBadge } from "@/components/StatusBadge";
import type {
  ReviewQueuePrediction,
  ReviewQueuePredictionStatus,
} from "@/lib/api";
import { cn } from "@/lib/utils";

export function PredictionStatusBadge({ className }: { className?: string }) {
  return (
    <StatusBadge
      color="primary"
      size="sm"
      outline
      className={cn("flex-none whitespace-nowrap !rounded-sm", className)}
    >
      <Sparkle aria-hidden className="size-3.5" />
      <span className="font-sans text-body-sm font-semibold tracking-normal">
        Judged
      </span>
    </StatusBadge>
  );
}

export function PredictionStatusIndicator({
  prediction,
  status,
  className,
}: {
  prediction: ReviewQueuePrediction | null;
  status: ReviewQueuePredictionStatus;
  className?: string;
}) {
  if (!prediction && (status === "queued" || status === "in_progress")) {
    return (
      <StatusBadge
        color="primary"
        size="sm"
        indicator
        spinning
        className={cn(
          "flex-none whitespace-nowrap !rounded-sm !border-transparent",
          className,
        )}
      >
        <span className="font-sans text-body-sm font-semibold tracking-normal">
          Judging…
        </span>
      </StatusBadge>
    );
  }

  if (!prediction && status === "failed") {
    return (
      <StatusBadge
        color="warning"
        size="sm"
        outline
        className={cn("flex-none whitespace-nowrap !rounded-sm", className)}
      >
        <TriangleAlert aria-hidden className="size-3.5" />
        <span className="font-sans text-body-sm font-semibold tracking-normal">
          Couldn’t judge
        </span>
      </StatusBadge>
    );
  }

  if (!prediction) {
    return (
      <StatusBadge
        color="muted"
        size="sm"
        outline
        className={cn(
          "flex-none whitespace-nowrap !rounded-sm border-dashed",
          className,
        )}
      >
        <Sparkle aria-hidden className="size-3.5" />
        <span className="font-sans text-body-sm font-semibold tracking-normal">
          Not judged
        </span>
      </StatusBadge>
    );
  }

  return <PredictionStatusBadge className={className} />;
}
