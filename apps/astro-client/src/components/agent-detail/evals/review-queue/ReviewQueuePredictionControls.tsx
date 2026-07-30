import { useRef } from "react";
import { Check, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type {
  DatasetJudgmentVerdict,
  ReviewQueuePrediction,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  PredictionVerdictBadge,
  predictionVerdict,
} from "./PredictionVerdictIndicator";
import {
  REVIEW_QUEUE_VERDICT_OPTIONS,
  ShortcutKey,
  useReviewQueueVerdictShortcuts,
} from "./ReviewQueueVerdictControls";

export function ReviewQueuePredictionControls({
  prediction,
  isPending,
  activeVerdict,
  showError,
  explanationOpen,
  onExplanationOpenChange,
  onSelect,
}: {
  prediction: ReviewQueuePrediction;
  isPending: boolean;
  activeVerdict: DatasetJudgmentVerdict | null;
  showError: boolean;
  explanationOpen: boolean;
  onExplanationOpenChange: (open: boolean) => void;
  onSelect: (
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLElement | null,
    agreesWithJudge: boolean,
  ) => void;
}) {
  const agreeButtonRef = useRef<HTMLButtonElement | null>(null);
  const predictedVerdict = predictionVerdict(prediction.verdict_score);
  const locked = isPending || activeVerdict !== null;
  const alternatives = REVIEW_QUEUE_VERDICT_OPTIONS.filter(
    ({ verdict }) => verdict !== predictedVerdict,
  );

  useReviewQueueVerdictShortcuts({
    disabled: locked,
    onSelect: (verdict) =>
      onSelect(
        verdict,
        verdict === predictedVerdict ? agreeButtonRef.current : null,
        false,
      ),
  });

  return (
    <div className="flex w-full flex-col gap-3">
      {showError && (
        <div className="text-body-sm text-destructive">
          Could not save verdict. Try again.
        </div>
      )}
      <div className="flex w-full flex-col gap-3 @[1040px]/review-card:flex-row @[1040px]/review-card:items-center @[1040px]/review-card:justify-between">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <PredictionVerdictBadge verdict={predictedVerdict} />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-expanded={explanationOpen}
            onClick={() => onExplanationOpenChange(!explanationOpen)}
            className="px-2 text-body-sm font-medium"
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

        <div className="flex flex-none items-center gap-2">
          <Button
            ref={agreeButtonRef}
            type="button"
            variant="outline"
            size="sm"
            disabled={locked}
            onClick={(event) =>
              onSelect(predictedVerdict, event.currentTarget, true)
            }
          >
            <Check aria-hidden className="size-4" />
            Agree with judge
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" variant="outline" size="sm" disabled={locked}>
                Disagree
                <ChevronDown aria-hidden className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuLabel className="text-body-sm text-muted-foreground">
                Mark as instead
              </DropdownMenuLabel>
              {alternatives.map(
                ({ verdict, label, shortcut, Icon, iconClassName }) => (
                  <DropdownMenuItem
                    key={verdict}
                    onSelect={(event) =>
                      onSelect(
                        verdict,
                        event.currentTarget as HTMLElement,
                        false,
                      )
                    }
                    className="py-2.5 text-body"
                  >
                    <Icon aria-hidden className={iconClassName} />
                    {label}
                    <ShortcutKey ariaHidden className="ml-auto">
                      {shortcut}
                    </ShortcutKey>
                  </DropdownMenuItem>
                ),
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>
  );
}
