import { useState } from "react";
import { useNavigate } from "react-router";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { DestructiveConfirmCheckbox } from "@/components/ui/destructive-confirm-checkbox";
import {
  useAccountMembers,
  useUpdateMemberRole,
  useRemoveAccountMember,
} from "@/api/queries";
import { useAuth } from "@/lib/auth";

interface LeaveOrganizationDialogProps {
  orgSlug: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function LeaveOrganizationDialog({
  orgSlug,
  open,
  onOpenChange,
}: LeaveOrganizationDialogProps) {
  const { user, accounts, refresh } = useAuth();
  const navigate = useNavigate();
  const { data: membersData, isLoading } = useAccountMembers(open ? orgSlug : "");
  const updateRole = useUpdateMemberRole();
  const removeMember = useRemoveAccountMember();
  const [selectedUserId, setSelectedUserId] = useState("");
  const [confirmed, setConfirmed] = useState(false);

  const org = accounts.find((a) => a.name === orgSlug);
  const members = membersData?.members ?? [];
  const currentUserId = user?.id ?? "";
  const currentMember = members.find((m) => m.user_id === currentUserId);
  const isAdmin = currentMember?.role === "admin" || currentMember?.role === "owner";
  const otherAdmins = members.filter(
    (m) => m.user_id !== currentUserId && (m.role === "admin" || m.role === "owner"),
  );
  const otherMembers = members.filter((m) => m.user_id !== currentUserId);
  const isLastAdmin = isAdmin && otherAdmins.length === 0;
  const isSoleMember = members.length <= 1;

  const isPending = updateRole.isPending || removeMember.isPending;
  const error = updateRole.isError
    ? (updateRole.error as Error)
    : removeMember.isError
      ? (removeMember.error as Error)
      : null;

  const handleReset = () => {
    setSelectedUserId("");
    setConfirmed(false);
    updateRole.reset();
    removeMember.reset();
  };

  const handleOpenChange = (o: boolean) => {
    onOpenChange(o);
    if (!o) handleReset();
  };

  const handleLeave = () => {
    if (!user) return;

    const doLeave = () => {
      removeMember.mutate(
        { account: orgSlug, userId: user.id },
        {
          onSuccess: () => {
            refresh();
            navigate("/settings/organizations");
          },
        },
      );
    };

    // If last admin, promote selected member first, then leave
    if (isLastAdmin && selectedUserId) {
      updateRole.mutate(
        { account: orgSlug, userId: selectedUserId, role: "admin" },
        { onSuccess: doLeave },
      );
    } else {
      doLeave();
    }
  };

  const canConfirm = (() => {
    if (!user || isSoleMember) return false;
    if (isLastAdmin) return !!selectedUserId && confirmed;
    return confirmed;
  })();

  const orgDisplayName = org?.display_name || orgSlug;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Leave organization</DialogTitle>
          <DialogDescription>
            {isSoleMember ? (
              <>
                You are the only member of <strong>{orgDisplayName}</strong>.
                You cannot leave the organization — you must delete it instead.
              </>
            ) : isLastAdmin ? (
              <>
                You are the last admin of <strong>{orgDisplayName}</strong>.
                Select a member to promote to admin before leaving.
              </>
            ) : (
              <>
                Are you sure you want to leave{" "}
                <strong>{orgDisplayName}</strong>? You will lose access to all
                organization agents and resources.
              </>
            )}
          </DialogDescription>
        </DialogHeader>

        {isLoading && (
          <div className="flex justify-center py-4">
            <Loader2 size={16} className="animate-spin text-muted-foreground" />
          </div>
        )}

        {!isLoading && isLastAdmin && !isSoleMember && (
          <div>
            <Label size="md">New admin</Label>
            <Select value={selectedUserId} onValueChange={setSelectedUserId}>
              <SelectTrigger>
                <SelectValue placeholder="Select a member" />
              </SelectTrigger>
              <SelectContent>
                {otherMembers.map((m) => (
                  <SelectItem key={m.user_id} value={m.user_id}>
                    {m.user_id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {!isLoading && !isSoleMember && (
          <DestructiveConfirmCheckbox checked={confirmed} onChange={setConfirmed}>
            I understand that I will lose access to all organization agents and
            resources.
          </DestructiveConfirmCheckbox>
        )}

        {error && (
          <p className="text-[13px] text-destructive">
            {error.message || "Failed to leave organization."}
          </p>
        )}

        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">Cancel</Button>
          </DialogClose>
          {isSoleMember ? (
            <Button
              variant="destructive"
              onClick={() => {
                handleOpenChange(false);
                // User needs to go delete the org — we just close the dialog
              }}
              disabled
            >
              Leave
            </Button>
          ) : (
            <Button
              variant="destructive"
              disabled={!canConfirm || isPending}
              onClick={handleLeave}
            >
              {isPending ? "Leaving..." : isLastAdmin ? "Transfer & leave" : "Leave"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
