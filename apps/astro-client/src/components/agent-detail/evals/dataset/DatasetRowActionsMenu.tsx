import { useRef, useState } from "react";
import { FilePen, MoreHorizontal, Trash2 } from "lucide-react";
import { InfoHint } from "@/components/InfoHint";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import type { EvaluationSetEvaluator, EvaluatorOutputValue } from "@/lib/api";
import { EvaluatorValueControl } from "../EvaluatorValueControl";
import { useEvaluatorOutputSelection } from "../useEvaluatorOutputSelection";

export interface DatasetRowActionsMenuProps {
  traceId: string;
  evaluators: EvaluationSetEvaluator[];
  editDisabled?: boolean;
  outdated?: boolean;
  savedOutputs: EvaluatorOutputValue[];
  isRemoving: boolean;
  isSavingOutputs: boolean;
  onRemove: (trigger: HTMLElement | null) => void;
  onSaveOutputs: (
    traceId: string,
    outputs: EvaluatorOutputValue[],
    onSaved: () => void,
  ) => void;
}

export function DatasetRowActionsMenu({
  traceId,
  evaluators,
  editDisabled = false,
  outdated = false,
  savedOutputs,
  isRemoving,
  isSavingOutputs,
  onRemove,
  onSaveOutputs,
}: DatasetRowActionsMenuProps) {
  const busy = isRemoving || isSavingOutputs;

  const [editing, setEditing] = useState(false);
  const firstControlRef = useRef<HTMLButtonElement | null>(null);
  const editItem = (
    <DropdownMenuItem
      disabled={editDisabled || outdated || busy}
      onSelect={() => setEditing(true)}
    >
      <FilePen className="size-4" />
      Edit evaluations
    </DropdownMenuItem>
  );

  return (
    <>
      <DropdownMenu>
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
        <DropdownMenuContent align="end">
          {outdated ? (
            <TooltipProvider delayDuration={300}>
              <Tooltip>
                <TooltipTrigger asChild>
                  {/* A disabled item takes no pointer events. */}
                  <span className="block">{editItem}</span>
                </TooltipTrigger>
                <TooltipContent className="max-w-xs">
                  This item was added with an old evaluator. Remove it from the
                  dataset and evaluate the trace again.
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ) : (
            editItem
          )}
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
      <Dialog open={editing} onOpenChange={setEditing}>
        <DialogContent
          showCloseButton={false}
          className="gap-0 overflow-hidden p-0 sm:max-w-md"
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            firstControlRef.current?.focus();
          }}
        >
          <OutputEditor
            evaluators={evaluators}
            savedOutputs={savedOutputs}
            isSaving={isSavingOutputs}
            disabled={busy}
            onSave={(outputs, onSaved) => onSaveOutputs(traceId, outputs, onSaved)}
            onClose={() => setEditing(false)}
            firstControlRef={firstControlRef}
          />
        </DialogContent>
      </Dialog>
    </>
  );
}

function OutputEditor({
  evaluators,
  savedOutputs,
  isSaving,
  disabled,
  onSave,
  onClose,
  firstControlRef,
}: {
  evaluators: EvaluationSetEvaluator[];
  savedOutputs: EvaluatorOutputValue[];
  isSaving: boolean;
  disabled: boolean;
  onSave: (outputs: EvaluatorOutputValue[], onSaved: () => void) => void;
  onClose: () => void;
  firstControlRef: React.Ref<HTMLButtonElement>;
}) {
  const { values, setValue, outputs, editedKeys } =
    useEvaluatorOutputSelection(evaluators, savedOutputs);

  return (
    <>
      <div className="border-b border-border bg-muted px-5 py-3.5">
        <DialogTitle className="truncate text-heading-4 font-semibold text-foreground">
          Edit evaluator values
        </DialogTitle>
        <DialogDescription className="sr-only">
          Change the verified values on this dataset item.
        </DialogDescription>
      </div>
      <div className="flex flex-col gap-3 px-5 py-4">
        <TooltipProvider delayDuration={300}>
          <div className="flex flex-col gap-3">
            {evaluators.map((evaluator, index) => (
              <div
                key={evaluator.key}
                className="flex items-center justify-between gap-2"
              >
                <span className="flex min-w-0 flex-1 items-center gap-1.5 text-body-sm font-medium text-foreground">
                  <span className="truncate">{evaluator.label}</span>
                  {evaluator.description && (
                    <InfoHint label={`About ${evaluator.label}`}>
                      {evaluator.description}
                    </InfoHint>
                  )}
                </span>
                <EvaluatorValueControl
                  output={evaluator.output}
                  label={evaluator.label}
                  value={values.get(evaluator.key)}
                  disabled={disabled}
                  controlRef={index === 0 ? firstControlRef : undefined}
                  onChange={(value) => setValue(evaluator.key, value)}
                />
              </div>
            ))}
          </div>
        </TooltipProvider>
        <div className="flex items-center justify-end gap-3 pt-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={isSaving}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            type="button"
            size="sm"
            onClick={() =>
              editedKeys.size === 0 ? onClose() : onSave(outputs, onClose)
            }
            disabled={disabled}
          >
            {isSaving ? "Saving..." : "Save"}
          </Button>
        </div>
      </div>
    </>
  );
}
