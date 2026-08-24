import { useId, type ReactNode } from "react";
import { Sparkle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Coachmark } from "@/components/ui/coachmark";

export function AutoEvaluateOnboardingCoachmark({
  open,
  anchor,
  onDismiss,
}: {
  open: boolean;
  anchor: ReactNode;
  onDismiss: () => void;
}) {
  const titleId = useId();
  const descriptionId = useId();

  return (
    <Coachmark
      open={open}
      anchor={anchor}
      sideOffset={12}
      className="w-[calc(100vw-2rem)] max-w-2xs"
      contentClassName="rounded-2xl p-4 shadow-xl"
      aria-labelledby={titleId}
      aria-describedby={descriptionId}
    >
      <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-3">
        <div className="flex size-8 items-center justify-center rounded-sm bg-primary/30 text-primary-foreground">
          <Sparkle aria-hidden className="size-4" />
        </div>
        <div className="min-w-0">
          <h2
            id={titleId}
            className="text-heading-4 font-semibold text-foreground"
          >
            Save time with auto-evaluation
          </h2>
          <p
            id={descriptionId}
            className="mt-2 text-body-sm text-muted-foreground"
          >
            Score every trace in one pass, then use the results while deciding
            which traces belong in the dataset.
          </p>
          <div className="mt-4 flex justify-end">
            <Button type="button" size="xs" onClick={onDismiss}>
              Got it
            </Button>
          </div>
        </div>
      </div>
    </Coachmark>
  );
}
