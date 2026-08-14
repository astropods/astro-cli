import { useState } from "react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import { Check, MoreHorizontal, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SelectableChip } from "@/components/ui/SelectableChip";
import type { JudgmentCriterion } from "@/lib/api";
import { JUDGMENT_CRITERIA, criterionOptions } from "../judgment-criteria";
import { useJudgmentCriteriaSelection } from "../useJudgmentCriteriaSelection";

export interface DatasetRowActionsMenuProps {
  traceId: string;
  savedCriteria: JudgmentCriterion[];
  isRemoving: boolean;
  isSavingCriteria: boolean;
  onRemove: (trigger: HTMLElement | null) => void;
  onSaveCriteria: (
    traceId: string,
    criteria: JudgmentCriterion[],
    onSaved: () => void,
  ) => void;
}

/** The three-dot actions menu for editing criteria or removing an item. */
export function DatasetRowActionsMenu({
  traceId,
  savedCriteria,
  isRemoving,
  isSavingCriteria,
  onRemove,
  onSaveCriteria,
}: DatasetRowActionsMenuProps) {
  const busy = isRemoving || isSavingCriteria;

  const [open, setOpen] = useState(false);

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Trace actions"
          disabled={busy}
          className="text-muted-foreground hover:text-foreground"
        >
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-96">
        <MenuCriteriaEditor
          key={savedCriteria
            .map(({ dimension_key, value }) => `${dimension_key}:${value}`)
            .join("|")}
          traceId={traceId}
          savedCriteria={savedCriteria}
          isSaving={isSavingCriteria}
          disabled={busy}
          onSaveCriteria={onSaveCriteria}
          onSaved={() => setOpen(false)}
        />
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          disabled={busy}
          onSelect={(event) =>
            onRemove(
              event.currentTarget instanceof HTMLElement
                ? event.currentTarget
                : null,
            )
          }
        >
          <Trash2 className="size-4" />
          Remove from dataset
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Criteria pills + Save inside the row menu. */
function MenuCriteriaEditor({
  traceId,
  savedCriteria,
  isSaving,
  disabled,
  onSaveCriteria,
  onSaved,
}: {
  traceId: string;
  savedCriteria: JudgmentCriterion[];
  isSaving: boolean;
  disabled: boolean;
  onSaveCriteria: (
    traceId: string,
    criteria: JudgmentCriterion[],
    onSaved: () => void,
  ) => void;
  onSaved: () => void;
}) {
  const { selected, dirty, toggle, criteria } =
    useJudgmentCriteriaSelection(savedCriteria);

  return (
    <>
      <DropdownMenuLabel className="text-faint-foreground">
        Evaluate item
      </DropdownMenuLabel>
      <div className="px-2 pb-1.5 pt-0.5">
        <div className="flex flex-col gap-2.5">
          {JUDGMENT_CRITERIA.map((dimension) => {
            return (
              <div
                key={dimension.dimensionKey}
                className="flex flex-col items-start gap-1.5"
              >
                <span className="text-body-sm font-medium text-foreground">
                  {dimension.dimensionLabel}
                </span>
                <div className="flex flex-wrap gap-2">
                  {criterionOptions(dimension).map(({ value, label }) => {
                    const isSelected =
                      selected.get(dimension.dimensionKey) === value;
                    return (
                      <DropdownMenuPrimitive.Item
                        key={value}
                        asChild
                        disabled={disabled}
                        onSelect={(event) => event.preventDefault()}
                      >
                        <SelectableChip
                          selected={isSelected}
                          tone="primary"
                          disabled={disabled}
                          onClick={() => toggle(dimension.dimensionKey, value)}
                          className="h-7 hover:ring-0!"
                        >
                          {label}
                          {isSelected && <Check aria-hidden className="size-4" />}
                        </SelectableChip>
                      </DropdownMenuPrimitive.Item>
                    );
                  })}
                </div>
              </div>
            );
          })}
        </div>
        {(dirty || isSaving) && (
          <DropdownMenuPrimitive.Item
            asChild
            disabled={disabled}
            onSelect={(event) => event.preventDefault()}
          >
            <Button
              type="button"
              size="sm"
              onClick={() => onSaveCriteria(traceId, criteria, onSaved)}
              disabled={disabled}
              className="mt-3 w-full"
            >
              {isSaving ? "Saving..." : "Save"}
            </Button>
          </DropdownMenuPrimitive.Item>
        )}
      </div>
    </>
  );
}
