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
import { JUDGMENT_CRITERIA, criterionOptions } from "../judgment-criteria";
import { useJudgmentCriteriaSelection } from "../useJudgmentCriteriaSelection";

export interface JudgmentCriteriaPanelProps {
  isUndoing: boolean;
  isSaving: boolean;
  isError: boolean;
  initialCriteria?: JudgmentCriterion[];
  onUndo: () => void;
  onSave: (criteria: JudgmentCriterion[]) => void;
}

/** Popup that lets the reviewer evaluate an added trace before saving. */
export function JudgmentCriteriaPanel({
  isUndoing,
  isSaving,
  isError,
  initialCriteria,
  onUndo,
  onSave,
}: JudgmentCriteriaPanelProps) {
  const firstCriterionRef = useRef<HTMLButtonElement | null>(null);
  const { selected, toggle, criteria } =
    useJudgmentCriteriaSelection(initialCriteria);

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
          <div className="flex min-w-0 items-center gap-2">
            <DialogTitle className="truncate text-heading-4 font-semibold text-foreground">
              Evaluate trace
            </DialogTitle>
            <InlineBadge className="text-label normal-case tracking-normal">
              Optional
            </InlineBadge>
          </div>
          <DialogDescription className="sr-only">
            Optionally select criteria for this trace, then save or undo.
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
          <div className="flex flex-col gap-3">
            {JUDGMENT_CRITERIA.map((dimension, index) => {
              return (
                <div
                  key={dimension.dimensionKey}
                  className="flex flex-wrap items-center gap-2"
                >
                  <span className="w-32 flex-none text-body-sm font-medium text-foreground">
                    {dimension.dimensionLabel}
                  </span>
                  {criterionOptions(dimension).map(
                    ({ value, label }, optionIndex) => {
                      const isSelected =
                        selected.get(dimension.dimensionKey) === value;
                      return (
                        <SelectableChip
                          key={value}
                          ref={
                            index === 0 && optionIndex === 0
                              ? firstCriterionRef
                              : undefined
                          }
                          selected={isSelected}
                          tone="primary"
                          disabled={isSaving || isUndoing}
                          onClick={() => toggle(dimension.dimensionKey, value)}
                          className="h-7"
                        >
                          {label}
                          {isSelected && (
                            <Check aria-hidden className="size-4" />
                          )}
                        </SelectableChip>
                      );
                    },
                  )}
                </div>
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
              onClick={() => onSave(criteria)}
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
