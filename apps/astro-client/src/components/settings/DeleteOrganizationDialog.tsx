import { useState } from "react";
import { useNavigate } from "react-router";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteAccount } from "@/api/queries";
import { useAuth } from "@/lib/auth";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface DeleteOrganizationDialogProps {
  orgSlug: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function DeleteOrganizationDialog({
  orgSlug,
  open,
  onOpenChange,
}: DeleteOrganizationDialogProps) {
  const { refresh } = useAuth();
  const [confirmation, setConfirmation] = useState("");
  const deleteAccount = useDeleteAccount();
  const navigate = useNavigate();

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Delete organization"
      description={
        <>
          This will permanently delete the organization, all its agents,
          deployments, and associated data.{" "}
          <span className="font-semibold text-destructive">
            This action cannot be undone.
          </span>
        </>
      }
      checkboxLabel={
        <>
          I understand that deleting this organization is{" "}
          <strong>irreversible</strong> and that all its content will be
          deleted forever.
        </>
      }
      actionLabel="Delete organization"
      pendingLabel="Deleting..."
      error={deleteAccount.isError ? (deleteAccount.error as Error) : null}
      defaultErrorMessage="Failed to delete organization."
      isPending={deleteAccount.isPending}
      canConfirm={confirmation === orgSlug}
      onConfirm={() => {
        deleteAccount.mutate(orgSlug, {
          onSuccess: () => {
            refresh();
            navigate("/settings/organizations");
          },
        });
      }}
      onReset={() => {
        setConfirmation("");
        deleteAccount.reset();
      }}
    >
      <div>
        <Label size="md">
          Type <span className="font-mono font-semibold">{orgSlug}</span> to
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
