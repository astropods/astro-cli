import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useArchiveBlueprint } from "@/api/queries/blueprints";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface ArchiveBlueprintDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  blueprintName: string;
  account: string;
  /** Called after the agent is successfully archived. */
  onArchived?: () => void;
}

export function ArchiveBlueprintDialog({
  open,
  onOpenChange,
  blueprintName,
  account,
  onArchived,
}: ArchiveBlueprintDialogProps) {
  const [confirmation, setConfirmation] = useState("");
  const archiveAgent = useArchiveBlueprint(account);

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Archive ${blueprintName}`}
      description={
        <>
          This will archive the{" "}
          <span className="font-semibold text-foreground">{blueprintName}</span> blueprint
          and hide it from listings. Existing deployments will not be affected.
        </>
      }
      checkboxLabel={
        <>
          I understand that archiving this blueprint will hide it from the
          catalog and account profile.
        </>
      }
      actionLabel="Archive blueprint"
      pendingLabel="Archiving…"
      error={archiveAgent.isError ? (archiveAgent.error as Error) : null}
      defaultErrorMessage="Failed to archive blueprint."
      isPending={archiveAgent.isPending}
      canConfirm={confirmation === blueprintName}
      onConfirm={() => {
        archiveAgent.mutate(
          { name: blueprintName },
          { onSuccess: () => { onOpenChange(false); onArchived?.(); } },
        );
      }}
      onReset={() => {
        setConfirmation("");
        archiveAgent.reset();
      }}
    >
      <div>
        <Label size="md">
          Type{" "}
          <span className="font-semibold">&ldquo;{blueprintName}&rdquo;</span> to
          confirm
        </Label>
        <Input
          value={confirmation}
          onChange={(e) => setConfirmation(e.target.value)}
          placeholder={blueprintName}
          autoComplete="off"
        />
      </div>
    </ConfirmationDialog>
  );
}
