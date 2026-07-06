import { useState, type ReactNode } from "react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import { Check, MoreHorizontal, Trash2, X } from "lucide-react";
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
import { cn } from "@/lib/utils";
import type {
  DatasetJudgmentVerdict,
  EvalDatasetItemsVerdict,
  JudgmentCriterion,
} from "@/lib/api";
import type { Verdict } from "./DatasetItemRow";
import { JUDGMENT_CRITERIA, criterionLabel } from "./judgment-criteria";
import { useJudgmentCriteriaSelection } from "./useJudgmentCriteriaSelection";

export interface DatasetRowActionsMenuProps {
  traceId: string;
  verdict: Verdict;
  savedCriteriaKeys: string[];
  isChanging: boolean;
  isRemoving: boolean;
  isSavingCriteria: boolean;
  onChangeVerdict: (traceId: string, verdict: DatasetJudgmentVerdict) => void;
  onRemoveVerdict: (traceId: string, trigger: HTMLElement | null) => void;
  onSaveCriteria: (
    traceId: string,
    criteria: JudgmentCriterion[],
    onSaved: () => void,
  ) => void;
}

/** The three-dot actions menu for a dataset item: change verdict, edit judgment
 *  criteria, or remove from the dataset. */
export function DatasetRowActionsMenu({
  traceId,
  verdict,
  savedCriteriaKeys,
  isChanging,
  isRemoving,
  isSavingCriteria,
  onChangeVerdict,
  onRemoveVerdict,
  onSaveCriteria,
}: DatasetRowActionsMenuProps) {
  const busy = isChanging || isRemoving || isSavingCriteria;
  const hasCriteria = verdict === "good" || verdict === "bad";

  const [open, setOpen] = useState(false);

  const verdictOptions: {
    value: DatasetJudgmentVerdict;
    label: string;
    icon: ReactNode;
    active: boolean;
    activeClass: string;
  }[] = [
    {
      value: "good",
      label: "Good",
      icon: <Check className="size-4 text-success" />,
      active: verdict === "good",
      activeClass: "bg-success/15 text-success focus:bg-success/20",
    },
    {
      value: "bad",
      label: "Bad",
      icon: <X className="size-4 text-destructive" />,
      active: verdict === "bad",
      activeClass: "bg-destructive/15 text-destructive focus:bg-destructive/20",
    },
  ];

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
      <DropdownMenuContent align="end" className={hasCriteria ? "w-80" : "w-52"}>
        <DropdownMenuLabel className="text-faint-foreground">
          Change verdict
        </DropdownMenuLabel>
        {verdictOptions.map((option) => (
          <DropdownMenuItem
            key={option.value}
            disabled={busy}
            onSelect={(event) => {
              // Keep the menu open so the reviewer can also adjust criteria.
              event.preventDefault();
              if (!option.active) onChangeVerdict(traceId, option.value);
            }}
            className={cn(option.active && option.activeClass)}
          >
            {option.icon}
            {option.label}
            {option.active && <Check className="ml-auto size-4 text-current" />}
          </DropdownMenuItem>
        ))}
        {hasCriteria && (
          <MenuCriteriaEditor
            key={verdict}
            traceId={traceId}
            verdict={verdict}
            savedCriteriaKeys={savedCriteriaKeys}
            isSaving={isSavingCriteria}
            disabled={busy}
            onSaveCriteria={onSaveCriteria}
            onSaved={() => setOpen(false)}
          />
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          disabled={busy}
          onSelect={(event) =>
            onRemoveVerdict(
              traceId,
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

/** Criteria pills + Save inside the row menu. Keyed by verdict by the caller so
 *  it remounts — resetting the selection — when the verdict changes (which
 *  clears the saved criteria) or when the menu reopens. */
function MenuCriteriaEditor({
  traceId,
  verdict,
  savedCriteriaKeys,
  isSaving,
  disabled,
  onSaveCriteria,
  onSaved,
}: {
  traceId: string;
  verdict: EvalDatasetItemsVerdict;
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
  const { selected, dirty, toggle, selectedCriteriaForVerdict } =
    useJudgmentCriteriaSelection(savedCriteriaKeys);

  const handleSave = () => {
    onSaveCriteria(traceId, selectedCriteriaForVerdict(verdict), onSaved);
  };

  return (
    <>
      <DropdownMenuSeparator />
      <DropdownMenuLabel className="text-faint-foreground">
        Why is it {verdict}?
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
                  {criterionLabel(dimension, verdict)}
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
