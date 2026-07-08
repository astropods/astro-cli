import { useEffect, useRef, type MouseEvent, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Check, Minus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { DatasetJudgmentVerdict } from "@/lib/api";

const REVIEW_QUEUE_VERDICT_OPTIONS: Array<{
  verdict: DatasetJudgmentVerdict;
  label: string;
  shortcut: string;
  Icon: LucideIcon;
  iconClassName: string;
}> = [
  {
    verdict: "good",
    label: "Good",
    shortcut: "G",
    Icon: Check,
    iconClassName: "text-success",
  },
  {
    verdict: "bad",
    label: "Bad",
    shortcut: "B",
    Icon: X,
    iconClassName: "text-destructive",
  },
  {
    verdict: "unknown",
    label: "Skip",
    shortcut: "S",
    Icon: Minus,
    iconClassName: "text-muted-foreground",
  },
];

const REVIEW_QUEUE_VERDICT_SHORTCUTS: Record<string, DatasetJudgmentVerdict> = {
  g: "good",
  b: "bad",
  s: "unknown",
};

// Highlight for the verdict already chosen; overrides the disabled dimming so
// the selection stays vivid while the other buttons grey out.
const VERDICT_ACTIVE_CLASS: Record<DatasetJudgmentVerdict, string> = {
  good: "border-success/50 bg-success/15 text-success disabled:opacity-100",
  bad: "border-destructive/50 bg-destructive/15 text-destructive disabled:opacity-100",
  unknown: "border-border-strong bg-muted text-foreground disabled:opacity-100",
};

function getReviewQueueShortcutVerdict(event: KeyboardEvent) {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    isEditableShortcutTarget(event.target)
  ) {
    return null;
  }

  return REVIEW_QUEUE_VERDICT_SHORTCUTS[event.key.toLowerCase()] ?? null;
}

function isEditableShortcutTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  return (
    target.isContentEditable ||
    Boolean(
      target.closest(
        "input, textarea, select, [contenteditable='true'], [contenteditable='plaintext-only']",
      ),
    )
  );
}

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
    trigger: HTMLButtonElement | null,
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

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const verdict = getReviewQueueShortcutVerdict(event);
      if (!verdict || locked) {
        return;
      }

      event.preventDefault();
      onSelect(verdict, verdictButtonRefs.current[verdict]);
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [locked, onSelect]);

  return (
    <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 @max-[520px]/review-card:flex-col @max-[520px]/review-card:items-stretch">
      {showError && (
        <div className="min-w-0 text-body-sm text-muted-foreground @max-[520px]/review-card:flex-none">
          Could not save verdict. Try again.
        </div>
      )}
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
                <ShortcutKey ariaHidden>{shortcut}</ShortcutKey>
              </Button>
            );
          },
        )}
      </div>
    </div>
  );
}

export function markedLabel(verdict: DatasetJudgmentVerdict) {
  const label =
    REVIEW_QUEUE_VERDICT_OPTIONS.find((option) => option.verdict === verdict)
      ?.label ?? "verdict";
  return `Marked as ${label.toLowerCase()}`;
}

function ShortcutKey({
  children,
  className,
  ariaHidden = false,
}: {
  children: ReactNode;
  className?: string;
  ariaHidden?: boolean;
}) {
  return (
    <kbd
      aria-hidden={ariaHidden || undefined}
      className={cn(
        "inline-flex h-5 min-w-5 items-center justify-center rounded border border-border-strong bg-muted/40 px-1.5 font-mono text-[11px] font-medium leading-none text-muted-foreground",
        className,
      )}
    >
      {children}
    </kbd>
  );
}
