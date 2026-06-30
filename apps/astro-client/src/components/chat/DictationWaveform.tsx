import { TooltipIconButton } from "@/components/assistant-ui/tooltip-icon-button";
import { useMicLevels } from "@/lib/chat/use-mic-levels";
import { cn } from "@/lib/utils";
import { Check, X } from "lucide-react";
import type { FC } from "react";

/**
 * Audio-reactive dictation surface shown in place of the composer input while
 * the mic is live. Bars are driven by real microphone amplitude (see
 * useMicLevels) and scroll right-to-left; the left edge fades to dots as
 * samples age. Cancel discards the in-progress transcript, confirm keeps it in
 * the composer for review/send.
 */
export const DictationWaveform: FC<{
  onCancel: () => void;
  onConfirm: () => void;
}> = ({ onCancel, onConfirm }) => {
  const levels = useMicLevels(true);

  return (
    <div className="flex w-full items-center gap-2 px-1.75 py-1">
      <div
        className="flex h-8 flex-1 items-center justify-between gap-px overflow-hidden"
        aria-hidden
      >
        {levels.map((level, i) => (
          <span
            key={i}
            className={cn(
              "min-w-px flex-1 rounded-full bg-foreground transition-[height] duration-200",
              level < 0.04 ? "opacity-30" : "opacity-80",
            )}
            style={{ height: `${Math.max(8, level * 100)}%` }}
          />
        ))}
      </div>
      <span className="sr-only" role="status">
        Listening…
      </span>
      <TooltipIconButton
        tooltip="Cancel"
        side="bottom"
        type="button"
        variant="ghost"
        size="icon"
        className="size-8 shrink-0 rounded-full"
        aria-label="Cancel dictation"
        onClick={onCancel}
      >
        <X className="size-4" />
      </TooltipIconButton>
      <TooltipIconButton
        tooltip="Done"
        side="bottom"
        type="button"
        variant="default"
        size="icon"
        className="size-8 shrink-0 rounded-full"
        aria-label="Finish dictation"
        onClick={onConfirm}
      >
        <Check className="size-4" />
      </TooltipIconButton>
    </div>
  );
};
