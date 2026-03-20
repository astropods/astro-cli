import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteAgent } from "@/api/queries/agents";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface DeleteAgentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentName: string;
  account: string;
  /** Called after the agent is successfully deleted. */
  onDeleted?: () => void;
}

export function DeleteAgentDialog({
  open,
  onOpenChange,
  agentName,
  account,
  onDeleted,
}: DeleteAgentDialogProps) {
  const [confirmation, setConfirmation] = useState("");
  const deleteAgent = useDeleteAgent(account);

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Delete ${agentName}`}
      description={
        <>
          This will permanently delete the{" "}
          <span className="font-semibold text-destructive">{agentName}</span> blueprint
          and all its published versions.{" "}
          <span className="font-semibold text-destructive">
            This action cannot be undone.
          </span>
        </>
      }
      checkboxLabel={
        <>
          I understand that deleting this blueprint is{" "}
          <strong>irreversible</strong> and that all published versions will be
          removed.
        </>
      }
      actionLabel="Delete blueprint"
      pendingLabel="Deleting…"
      error={deleteAgent.isError ? (deleteAgent.error as Error) : null}
      defaultErrorMessage="Failed to delete blueprint."
      isPending={deleteAgent.isPending}
      canConfirm={confirmation === agentName}
      onConfirm={() => {
        deleteAgent.mutate(
          { name: agentName },
          { onSuccess: () => { onOpenChange(false); onDeleted?.(); } },
        );
      }}
      onReset={() => {
        setConfirmation("");
        deleteAgent.reset();
      }}
    >
      <div>
        <Label size="md">
          Type{" "}
          <span className="font-semibold">&ldquo;{agentName}&rdquo;</span> to
          confirm
        </Label>
        <Input
          value={confirmation}
          onChange={(e) => setConfirmation(e.target.value)}
          placeholder={agentName}
          autoComplete="off"
        />
      </div>
    </ConfirmationDialog>
  );
}
