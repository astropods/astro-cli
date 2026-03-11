import { useState } from "react";
import { useNavigate } from "react-router";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteAccount } from "@/api/queries";
import { useAuth } from "@/lib/auth";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface DeleteAccountDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function DeleteAccountDialog({
  open,
  onOpenChange,
}: DeleteAccountDialogProps) {
  const { personalAccount, logout } = useAuth();
  const [confirmation, setConfirmation] = useState("");
  const deleteAccount = useDeleteAccount();
  const navigate = useNavigate();

  const username = personalAccount?.name ?? "";

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Delete account"
      description={
        <>
          This will permanently delete your account, all your agents,
          deployments, and associated data.{" "}
          <span className="font-semibold text-destructive">
            This action cannot be undone.
          </span>
        </>
      }
      checkboxLabel={
        <>
          I understand that deleting my account is{" "}
          <strong>irreversible</strong> and that all my existing content
          will be deleted forever.
        </>
      }
      actionLabel="Delete account"
      pendingLabel="Deleting…"
      error={deleteAccount.isError ? (deleteAccount.error as Error) : null}
      defaultErrorMessage="Failed to delete account."
      isPending={deleteAccount.isPending}
      canConfirm={confirmation === username}
      onConfirm={() => {
        deleteAccount.mutate(username, {
          onSuccess: () => {
            logout();
            navigate("/");
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
          Type <span className="font-mono font-semibold">{username}</span> to
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
