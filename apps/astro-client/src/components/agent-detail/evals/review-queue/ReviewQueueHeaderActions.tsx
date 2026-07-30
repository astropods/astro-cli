import { ArrowRight, ChevronDown, ChevronUp } from "lucide-react";
import { Button } from "@/components/ui/button";

export function ReviewQueueHeaderActions({
  position,
  total,
  canGoPrevious,
  canGoNext,
  onPrevious,
  onNext,
  onOpenTrace,
  traceLabel,
}: {
  position: number;
  total: number;
  canGoPrevious: boolean;
  canGoNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
  onOpenTrace?: () => void;
  traceLabel: string;
}) {
  if (total <= 0) {
    return null;
  }

  return (
    <div className="flex items-center gap-1">
      <span
        className="mr-1 min-w-16 text-center font-mono text-mono-sm text-foreground tabular-nums"
        aria-label={`Trace ${position} of ${total}`}
      >
        {position} of {total}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={onPrevious}
        disabled={!canGoPrevious}
        aria-label="Previous trace"
      >
        <ChevronUp aria-hidden className="size-4" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={onNext}
        disabled={!canGoNext}
        aria-label="Next trace"
      >
        <ChevronDown aria-hidden className="size-4" />
      </Button>
      {onOpenTrace && (
        <>
          <span aria-hidden className="mx-2 h-8 w-px bg-border" />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onOpenTrace}
            aria-label={`View ${traceLabel}`}
            className="group/trace text-body-sm font-medium"
          >
            View trace
            <ArrowRight
              aria-hidden
              className="size-4 transition-transform group-hover/trace:translate-x-0.5"
            />
          </Button>
        </>
      )}
    </div>
  );
}
