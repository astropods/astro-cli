import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useArchiveAgent } from "@/api/queries/agents";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface ArchiveAgentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentName: string;
  account: string;
  /** Called after the agent is successfully archived. */
  onArchived?: () => void;
}

export function ArchiveAgentDialog({
  open,
  onOpenChange,
  agentName,
  account,
  onArchived,
}: ArchiveAgentDialogProps) {
  const [confirmation, setConfirmation] = useState("");
  const archiveAgent = useArchiveAgent(account);

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Archive ${agentName}`}
      description={
        <>
          This will archive the{" "}
          <span className="font-semibold text-destructive">{agentName}</span> blueprint
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
      canConfirm={confirmation === agentName}
      onConfirm={() => {
        archiveAgent.mutate(
          { name: agentName },
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
