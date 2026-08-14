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
import { JUDGMENT_CRITERIA } from "../judgment-criteria";
import { useJudgmentCriteriaSelection } from "../useJudgmentCriteriaSelection";

export interface DatasetRowActionsMenuProps {
  traceId: string;
  savedCriteriaKeys: string[];
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
  savedCriteriaKeys,
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
      <DropdownMenuContent align="end" className="w-80">
        <MenuCriteriaEditor
          key={savedCriteriaKeys.join("|")}
          traceId={traceId}
          savedCriteriaKeys={savedCriteriaKeys}
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
  savedCriteriaKeys,
  isSaving,
  disabled,
  onSaveCriteria,
  onSaved,
}: {
  traceId: string;
  savedCriteriaKeys: string[];
  isSaving: boolean;
  disabled: boolean;
  onSaveCriteria: (
    traceId: string,
    criteria: JudgmentCriterion[],
    onSaved: () => void,
  ) => void;
  onSaved: () => void;
}) {
  const { selected, dirty, toggle } =
    useJudgmentCriteriaSelection(savedCriteriaKeys);

  const handleSave = () => {
    onSaveCriteria(
      traceId,
      JUDGMENT_CRITERIA.filter(({ dimensionKey }) =>
        selected.has(dimensionKey),
      ).map(({ dimensionKey }) => ({
        dimension_key: dimensionKey,
        value: 1,
      })),
      onSaved,
    );
  };

  return (
    <>
      <DropdownMenuLabel className="text-faint-foreground">
        Evaluate item
      </DropdownMenuLabel>
      <div className="px-2 pb-1.5 pt-0.5">
        <div className="flex flex-wrap gap-2">
          {JUDGMENT_CRITERIA.map((dimension) => {
            const isSelected = selected.has(dimension.dimensionKey);
            return (
              // Wrap as a menu item so keyboard nav reaches it; onSelect keeps
              // the menu open, the pill's own click toggles.
              <DropdownMenuPrimitive.Item
                key={dimension.dimensionKey}
                asChild
                disabled={disabled}
                onSelect={(event) => event.preventDefault()}
              >
                <SelectableChip
                  selected={isSelected}
                  tone="primary"
                  disabled={disabled}
                  onClick={() => toggle(dimension.dimensionKey)}
                  // Drop the focus ring on hover (Radix focuses on hover);
                  // keyboard focus keeps it.
                  className="h-7 hover:ring-0!"
                >
                  {dimension.goodLabel}
                  {isSelected && <Check aria-hidden className="size-4" />}
                </SelectableChip>
              </DropdownMenuPrimitive.Item>
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
              onClick={handleSave}
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
