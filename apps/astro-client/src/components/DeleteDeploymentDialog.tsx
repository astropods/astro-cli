import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUndeployAgent } from "@/api/queries/deployments";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface DeleteDeploymentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  deploymentId: string;
  deploymentName: string;
  displayName?: string;
  account: string;
}

export function DeleteDeploymentDialog({
  open,
  onOpenChange,
  deploymentId,
  deploymentName,
  displayName,
  account,
}: DeleteDeploymentDialogProps) {
  const [confirmation, setConfirmation] = useState("");
  const undeploy = useUndeployAgent(account);

  const label = displayName || deploymentName;

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Delete ${label}`}
      description={
        <>
          This will permanently delete{" "}
          <span className="font-semibold text-destructive">{label}</span>, tear down
          all running resources, and remove associated data.{" "}
          <span className="font-semibold text-destructive">
            This action cannot be undone.
          </span>
        </>
      }
      checkboxLabel={
        <>
          I understand that deleting this deployment is{" "}
          <strong>irreversible</strong> and that all running resources will be
          destroyed.
        </>
      }
      actionLabel="Delete deployment"
      pendingLabel="Deleting…"
      error={undeploy.isError ? (undeploy.error as Error) : null}
      defaultErrorMessage="Failed to delete deployment."
      isPending={undeploy.isPending}
      canConfirm={confirmation === deploymentName}
      onConfirm={() => {
        undeploy.mutate(
          { deployment_id: deploymentId },
          { onSuccess: () => onOpenChange(false) },
        );
      }}
      onReset={() => {
        setConfirmation("");
        undeploy.reset();
      }}
    >
      <div>
        <Label size="md">
          Type{" "}
          <span className="font-mono font-semibold">{deploymentName}</span> to
          confirm
        </Label>
        <Input
          value={confirmation}
          onChange={(e) => setConfirmation(e.target.value)}
          autoComplete="off"
        />
      </div>
    </ConfirmationDialog>
  );
}
