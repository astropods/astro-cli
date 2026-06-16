import { useState } from "react";
import { Label } from "@/components/ui/label";
import { useRenameAccount } from "@/api/queries";
import { useAccountNameValidation } from "@/hooks/use-account-name";
import { AccountNameInput } from "@/components/AccountNameInput";
import { ConfirmationDialog } from "@/components/ConfirmationDialog";

interface ChangeUsernameDialogProps {
  currentName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

export function ChangeUsernameDialog({
  currentName,
  open,
  onOpenChange,
  onSuccess,
}: ChangeUsernameDialogProps) {
  const [newUsername, setNewUsername] = useState("");
  const renameAccount = useRenameAccount();
  const { isChecking, isAvailable, displayError } = useAccountNameValidation(
    open ? newUsername : "",
  );

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Change username"
      description={
        <>
          Changing your username will break any existing links or CLI
          configurations that reference your current name.
        </>
      }
      checkboxLabel={
        <>
          I understand that changing my username is a destructive action
          and any existing links to my content on Astro will no longer
          function.
        </>
      }
      actionLabel="Change username"
      pendingLabel="Changing…"
      error={renameAccount.isError ? (renameAccount.error as Error) : null}
      defaultErrorMessage="Failed to rename account."
      isPending={renameAccount.isPending}
      canConfirm={isAvailable}
      onConfirm={() => {
        renameAccount.mutate(
          { account: currentName, newName: newUsername.trim() },
          { onSuccess },
        );
      }}
      onReset={() => {
        setNewUsername("");
        renameAccount.reset();
      }}
    >
      <div>
        <Label size="md">New username</Label>
        <AccountNameInput
          value={newUsername}
          onChange={setNewUsername}
          placeholder={currentName}
          isChecking={isChecking}
          isAvailable={isAvailable}
          displayError={displayError}
        />
      </div>
    </ConfirmationDialog>
  );
}
