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
  onSuccess: (newName: string) => void;
  variant?: "personal" | "organization";
}

export function ChangeUsernameDialog({
  currentName,
  open,
  onOpenChange,
  onSuccess,
  variant = "personal",
}: ChangeUsernameDialogProps) {
  const [newUsername, setNewUsername] = useState("");
  const renameAccount = useRenameAccount();
  const { isChecking, isAvailable, displayError } = useAccountNameValidation(
    open ? newUsername : "",
  );

  const isOrg = variant === "organization";

  return (
    <ConfirmationDialog
      open={open}
      onOpenChange={onOpenChange}
      title={isOrg ? "Change organization username" : "Change username"}
      description={
        isOrg ? (
          <>
            Changing the username will break any existing links or CLI
            configurations that reference the current name.
          </>
        ) : (
          <>
            Changing your username will break any existing links or CLI
            configurations that reference your current name.
          </>
        )
      }
      checkboxLabel={
        isOrg ? (
          <>
            I understand that changing the username is a destructive action
            and any existing links to this organization on Astro will no
            longer function.
          </>
        ) : (
          <>
            I understand that changing my username is a destructive action
            and any existing links to my content on Astro will no longer
            function.
          </>
        )
      }
      actionLabel="Change username"
      pendingLabel="Changing…"
      error={renameAccount.isError ? (renameAccount.error as Error) : null}
      defaultErrorMessage={
        isOrg ? "Failed to rename organization." : "Failed to rename account."
      }
      isPending={renameAccount.isPending}
      canConfirm={isAvailable}
      onConfirm={() => {
        const trimmed = newUsername.trim();
        renameAccount.mutate(
          { account: currentName, newName: trimmed },
          { onSuccess: () => onSuccess(trimmed) },
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
