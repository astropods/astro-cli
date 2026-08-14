import { ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { PredictionStatusBadge } from "./PredictionStatusIndicator";

export function ReviewQueuePredictionControls({
  explanationOpen,
  onExplanationOpenChange,
}: {
  explanationOpen: boolean;
  onExplanationOpenChange: (open: boolean) => void;
}) {
  return (
    <div className="flex min-w-0 flex-nowrap items-center gap-2">
      <PredictionStatusBadge />
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
