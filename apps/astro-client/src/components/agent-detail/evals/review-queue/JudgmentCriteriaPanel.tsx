import { useRef } from "react";
import { Check } from "lucide-react";
import { InlineBadge } from "@/components/InlineBadge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { SelectableChip } from "@/components/ui/SelectableChip";
import type { JudgmentCriterion } from "@/lib/api";
import { JUDGMENT_CRITERIA, criterionLabel } from "../judgment-criteria";
import { useJudgmentCriteriaSelection } from "../useJudgmentCriteriaSelection";

export interface JudgmentCriteriaPanelProps {
  /** The verdict just recorded; drives the label set and chip tone. */
  verdict: "good" | "bad";
  /** Header label, e.g. "Marked as good". */
  title: string;
  isUndoing: boolean;
  isSaving: boolean;
  isError: boolean;
  initialKeys?: Iterable<string>;
  onUndo: () => void;
  onSave: (criteria: JudgmentCriterion[]) => void;
}

/** Popup that confirms a verdict with an Undo and lets the reviewer optionally
 *  pick judgment criteria before dismissing with Save. */
export function JudgmentCriteriaPanel({
  verdict,
  title,
  isUndoing,
  isSaving,
  isError,
  initialKeys,
  onUndo,
  onSave,
}: JudgmentCriteriaPanelProps) {
  const firstCriterionRef = useRef<HTMLButtonElement | null>(null);
  const { selected, toggle, selectedCriteriaForVerdict } =
    useJudgmentCriteriaSelection(initialKeys);

  const handleSave = () => {
    onSave(selectedCriteriaForVerdict(verdict));
  };

  return (
    <Dialog open modal={false}>
      <DialogContent
        showCloseButton={false}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          firstCriterionRef.current?.focus();
        }}
        className="bottom-6 top-auto w-[calc(100vw-2rem)] max-w-xl translate-y-0 gap-0 overflow-hidden border-border-strong bg-card p-0 text-foreground shadow-xl sm:max-w-xl dark:bg-background"
      >
        <div className="flex items-center justify-between gap-3 bg-muted px-5 py-2.5">
          <DialogTitle className="min-w-0 truncate text-body-sm font-normal text-foreground">
            {title}
          </DialogTitle>
          <DialogDescription className="sr-only">
            Optionally select criteria for this verdict, then save or undo it.
          </DialogDescription>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={isUndoing || isSaving}
            onClick={onUndo}
            className="h-7 flex-none px-3 font-semibold"
          >
            {isUndoing ? "Undoing..." : "Undo"}
          </Button>
        </div>

        <div className="flex flex-col gap-3 px-5 py-4">
          <div className="flex items-center gap-2">
            <h3 className="text-heading-4 font-semibold text-foreground">
              Why is it {verdict}?
            </h3>
            <InlineBadge className="text-label normal-case tracking-normal">
              Optional
            </InlineBadge>
          </div>

          <div className="flex flex-wrap gap-2">
            {JUDGMENT_CRITERIA.map((dimension, index) => {
              const isSelected = selected.has(dimension.dimensionKey);
              return (
                <SelectableChip
                  key={dimension.dimensionKey}
                  ref={index === 0 ? firstCriterionRef : undefined}
                  selected={isSelected}
                  tone={verdict === "bad" ? "destructive" : "success"}
                  disabled={isSaving || isUndoing}
                  onClick={() => toggle(dimension.dimensionKey)}
                  className="h-7"
                >
                  {criterionLabel(dimension, verdict)}
                  {isSelected && <Check aria-hidden className="size-4" />}
                </SelectableChip>
              );
            })}
          </div>

          <div className="flex items-center justify-end gap-3">
            {isError && (
              <span className="mr-auto text-body-sm text-muted-foreground">
                Could not save criteria. Try again.
              </span>
            )}
            <Button
              type="button"
              size="sm"
              onClick={handleSave}
              disabled={isSaving || isUndoing}
              className="h-7 px-3.5"
            >
              {isSaving ? "Saving..." : "Save"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
