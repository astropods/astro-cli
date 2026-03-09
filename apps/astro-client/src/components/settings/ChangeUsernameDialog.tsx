import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useRenameAccount } from "@/api/queries";
import { useAccountNameValidation, sanitizeAccountName } from "@/hooks/use-account-name";
import { cn } from "@/lib/utils";

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
  const [confirmed, setConfirmed] = useState(false);
  const renameAccount = useRenameAccount();
  const { isChecking, isAvailable, displayError } = useAccountNameValidation(
    open ? newUsername : "",
  );

  const handleOpenChange = (o: boolean) => {
    onOpenChange(o);
    if (!o) {
      setNewUsername("");
      setConfirmed(false);
      renameAccount.reset();
    }
  };

  const handleRename = () => {
    renameAccount.mutate(
      { account: currentName, newName: newUsername.trim() },
      { onSuccess },
    );
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button variant="link" className="h-auto p-0 text-[13px]">
          Change username
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Change username</DialogTitle>
          <DialogDescription>
            Changing your username will break any existing links or CLI
            configurations that reference your current name.
          </DialogDescription>
        </DialogHeader>
        <div>
          <label className="mb-1.5 block font-mono text-mono-md uppercase tracking-widest text-ink-muted">
            New username
          </label>
          <div className="relative">
            <Input
              value={newUsername}
              onChange={(e) => setNewUsername(sanitizeAccountName(e.target.value))}
              placeholder={currentName}
              maxLength={39}
              aria-invalid={!!displayError || undefined}
              className={cn(
                "pr-9",
                isAvailable && "border-green-600 focus-visible:border-green-600",
              )}
            />
            <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-lg leading-none">
              {newUsername.length === 0 ? "" : isChecking ? "\u2026" : isAvailable ? "\u2713" : displayError ? "\u2717" : ""}
            </span>
          </div>
          <div className="mt-1.5 min-h-5 text-xs">
            {newUsername.length > 0 && displayError && (
              <p className="text-destructive">{displayError}</p>
            )}
            {isChecking && (
              <p className="text-muted-foreground">Checking availability...</p>
            )}
            {isAvailable && (
              <p className="text-green-600">Available</p>
            )}
          </div>
        </div>
        <label className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 p-3 cursor-pointer">
          <input
            type="checkbox"
            checked={confirmed}
            onChange={(e) => setConfirmed(e.target.checked)}
            className="mt-0.5 accent-destructive"
          />
          <span className="text-[13px] leading-snug text-destructive">
            I understand that changing my username is a destructive action
            and any existing links to my content on Astro will no longer
            function.
          </span>
        </label>
        {renameAccount.isError && (
          <p className="text-[13px] text-destructive">
            {(renameAccount.error as Error)?.message || "Failed to rename account."}
          </p>
        )}
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          <Button
            variant="destructive"
            disabled={!confirmed || !isAvailable || renameAccount.isPending}
            onClick={handleRename}
          >
            {renameAccount.isPending ? "Changing…" : "Change username"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
