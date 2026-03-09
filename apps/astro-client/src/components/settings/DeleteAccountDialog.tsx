import { useState } from "react";
import { useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useDeleteAccount } from "@/api/queries";
import { useAuth } from "@/lib/auth";
import { DestructiveConfirmCheckbox } from "@/components/ui/destructive-confirm-checkbox";

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
  const [confirmed, setConfirmed] = useState(false);
  const deleteAccount = useDeleteAccount();
  const navigate = useNavigate();

  const username = personalAccount?.name ?? "";
  const usernameMatches = confirmation === username;

  const handleOpenChange = (o: boolean) => {
    onOpenChange(o);
    if (!o) {
      setConfirmation("");
      setConfirmed(false);
      deleteAccount.reset();
    }
  };

  const handleDelete = () => {
    deleteAccount.mutate(username, {
      onSuccess: () => {
        logout();
        navigate("/");
      },
    });
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Delete account</DialogTitle>
          <DialogDescription>
            This will permanently delete your account, all your agents,
            deployments, and associated data.{" "}
            <span className="font-semibold text-destructive">
              This action cannot be undone.
            </span>
          </DialogDescription>
        </DialogHeader>
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
        <DestructiveConfirmCheckbox checked={confirmed} onChange={setConfirmed}>
          I understand that deleting my account is{" "}
          <strong>irreversible</strong> and that all my existing content
          will be deleted forever.
        </DestructiveConfirmCheckbox>
        {deleteAccount.isError && (
          <p className="text-[13px] text-destructive">
            {(deleteAccount.error as Error)?.message ||
              "Failed to delete account."}
          </p>
        )}
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          <Button
            variant="destructive"
            disabled={!usernameMatches || !confirmed || deleteAccount.isPending}
            onClick={handleDelete}
          >
            {deleteAccount.isPending ? "Deleting…" : "Delete account"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
