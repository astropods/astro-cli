import { useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { InviteInput, type InviteEntry } from "@/components/InviteInput";
import { useCreateInvitations } from "@/api/queries";

interface InviteMembersDialogProps {
  orgSlug: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function InviteMembersDialog({
  orgSlug,
  open,
  onOpenChange,
}: InviteMembersDialogProps) {
  const [invites, setInvites] = useState<InviteEntry[]>([]);
  const createInvitations = useCreateInvitations();

  const validInvites = invites.filter((inv) => inv.valid);
  const canSend = validInvites.length > 0;

  const handleSend = () => {
    const invitations = validInvites.map((inv) => ({
      value: inv.value,
      kind: inv.kind,
      role: "member" as const,
    }));
    createInvitations.mutate(
      { account: orgSlug, invitations },
      {
        onSuccess: () => {
          setInvites([]);
          onOpenChange(false);
        },
      },
    );
  };

  const handleOpenChange = (o: boolean) => {
    onOpenChange(o);
    if (!o) {
      setInvites([]);
      createInvitations.reset();
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Invite members</DialogTitle>
          <DialogDescription>
            Invite people by email address or Astro username.
          </DialogDescription>
        </DialogHeader>
        <div>
          <Label size="md">Members</Label>
          <InviteInput
            entries={invites}
            onChange={setInvites}
            placeholder="Enter email or username"
          />
        </div>
        {createInvitations.isError && (
          <p className="text-[13px] text-destructive">
            {(createInvitations.error as Error)?.message ||
              "Failed to send invitations."}
          </p>
        )}
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          <Button
            disabled={!canSend || createInvitations.isPending}
            onClick={handleSend}
          >
            {createInvitations.isPending && (
              <Loader2 size={14} className="spinner-delayed" />
            )}
            {validInvites.length > 1
              ? `Send ${validInvites.length} invitations`
              : "Send invitation"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
