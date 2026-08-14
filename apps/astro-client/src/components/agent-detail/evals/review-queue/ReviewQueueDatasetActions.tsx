import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export function ReviewQueueDatasetActions({
  isPending,
  showError,
  onAdd,
  onRemove,
}: {
  isPending: boolean;
  showError: boolean;
  onAdd: (trigger: HTMLElement | null) => void;
  onRemove: (trigger: HTMLElement | null) => void;
}) {
  return (
    <div className="flex w-full flex-col gap-2">
      {showError && (
        <div className="text-body-sm text-destructive">
          Could not update the review queue. Try again.
        </div>
      )}
      <div className="flex flex-wrap items-center justify-end gap-2">
        <TooltipProvider delayDuration={300}>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={isPending}
                onClick={(event) => onRemove(event.currentTarget)}
              >
                Remove
              </Button>
            </TooltipTrigger>
            <TooltipContent>Remove from review queue</TooltipContent>
          </Tooltip>
        </TooltipProvider>
        <Button
          type="button"
          size="sm"
          disabled={isPending}
          onClick={(event) => onAdd(event.currentTarget)}
        >
          <Plus aria-hidden className="size-4" />
          Add to dataset
        </Button>
      </div>
    </div>
  );
}
