import { useRef, type MouseEvent } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { DatasetJudgmentVerdict } from "@/lib/api";
import { REVIEW_QUEUE_VERDICT_OPTIONS } from "./review-queue-verdict-options";
import { useReviewQueueShortcuts } from "./review-queue-shortcuts";
import { ReviewQueueShortcutKey } from "./ReviewQueueShortcutKey";

// Highlight for the verdict already chosen; overrides the disabled dimming so
// the selection stays vivid while the other buttons grey out.
const VERDICT_ACTIVE_CLASS: Record<DatasetJudgmentVerdict, string> = {
  good: "border-success/50 bg-success/15 text-success disabled:opacity-100",
  bad: "border-destructive/50 bg-destructive/15 text-destructive disabled:opacity-100",
  unknown: "border-border-strong bg-muted text-foreground disabled:opacity-100",
};

export function ReviewQueueVerdictControls({
  isPending,
  activeVerdict,
  showError,
  onSelect,
}: {
  isPending: boolean;
  activeVerdict: DatasetJudgmentVerdict | null;
  showError: boolean;
  onSelect: (
    verdict: DatasetJudgmentVerdict,
    trigger: HTMLElement | null,
  ) => void;
}) {
  const verdictButtonRefs = useRef<
    Record<DatasetJudgmentVerdict, HTMLButtonElement | null>
  >({
    good: null,
    bad: null,
    unknown: null,
  });
  // Once a verdict is recorded, lock the controls until the trace clears.
  const locked = isPending || activeVerdict !== null;

  useReviewQueueShortcuts({
    disabled: locked,
    onSelect: (verdict) =>
      onSelect(verdict, verdictButtonRefs.current[verdict]),
  });

  return (
    <div className="flex w-full flex-wrap items-center gap-x-4 gap-y-2 @max-[520px]/review-card:flex-col @max-[520px]/review-card:items-stretch">
      {showError && (
        <div className="w-full min-w-0 text-body-sm text-destructive @max-[520px]/review-card:flex-none">
          Could not save verdict. Try again.
        </div>
      )}
      <span className="flex-none text-body-sm text-muted-foreground">
        Select a verdict
      </span>
      <div className="flex flex-wrap items-center gap-2 @max-[520px]/review-card:grid @max-[520px]/review-card:grid-cols-1">
        {REVIEW_QUEUE_VERDICT_OPTIONS.map(
          ({ verdict, label, shortcut, Icon, iconClassName }) => {
            const isActive = activeVerdict === verdict;
            return (
              <Button
                key={verdict}
                ref={(node) => {
                  verdictButtonRefs.current[verdict] = node;
                }}
                type="button"
                variant="outline"
                size="sm"
                disabled={locked}
                aria-pressed={isActive}
                onClick={(event: MouseEvent<HTMLButtonElement>) =>
                  onSelect(verdict, event.currentTarget)
                }
                className={cn(
                  "@max-[520px]/review-card:w-full",
                  isActive && VERDICT_ACTIVE_CLASS[verdict],
                )}
              >
                <Icon className={cn("size-4", iconClassName)} />
                {label}
                <ReviewQueueShortcutKey ariaHidden>
                  {shortcut}
                </ReviewQueueShortcutKey>
              </Button>
            );
          },
        )}
      </div>
    </div>
  );
}
