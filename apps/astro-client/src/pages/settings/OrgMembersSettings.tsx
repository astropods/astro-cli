import { useState } from "react";
import { useParams } from "react-router";
import {
  MoreHorizontal,
  Loader2,
  Users,
  UserPlus,
  Trash2,
  ChevronDown,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Separator } from "@/components/ui/separator";
import { UserAvatar } from "@/components/UserAvatar";
import {
  useAccountMembers,
  useUpdateMemberRole,
  useRemoveAccountMember,
} from "@/api/queries";
import { useAuth } from "@/lib/auth";
import { formatRelativeTime } from "@/lib/deployment-utils";
import { InviteMembersDialog } from "@/components/settings/InviteMembersDialog";
import type { AccountMember } from "@/lib/api";

const GRID_COLS = "grid-cols-[1.5fr_0.75fr_0.75fr_56px]";

const ROLES = [
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
];

function RoleCell({
  role,
  isAdmin,
  canChange,
  onChangeRole,
}: {
  role: string;
  isAdmin: boolean;
  canChange: boolean;
  onChangeRole: (role: string) => void;
}) {
  const label = ROLES.find((r) => r.value === role)?.label ?? role;

  if (!isAdmin || !canChange) {
    return (
      <span className="text-[13px] text-foreground capitalize">{label}</span>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="inline-flex items-center gap-1 text-[13px] text-foreground capitalize cursor-pointer hover:text-muted-foreground transition-colors"
        >
          {label}
          <ChevronDown className="size-3 text-muted-foreground" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-[120px]">
        {ROLES.map((r) => (
          <DropdownMenuItem
            key={r.value}
            disabled={r.value === role}
            onSelect={() => onChangeRole(r.value)}
          >
            {r.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function MemberRow({
  member,
  isLast,
  isCurrentUser,
  isAdmin,
  onChangeRole,
  onRemove,
}: {
  member: AccountMember;
  isLast: boolean;
  isCurrentUser: boolean;
  isAdmin: boolean;
  onChangeRole: (role: string) => void;
  onRemove: () => void;
}) {
  const displayName = member.display_name || member.username || member.user_id;
  const isPending = member.status === "pending";

  return (
    <div
      className={`grid ${GRID_COLS} gap-x-3 px-4 items-center hover:bg-muted/40 transition-colors ${isLast ? "" : "border-b border-border"}`}
    >
      <div className="flex items-center gap-3 py-3 min-w-0 overflow-hidden">
        <UserAvatar
          handle={member.username || member.user_id}
          name={displayName}
          className="size-8 shrink-0"
        />
        <div className="min-w-0">
          <div className="text-[13px] font-medium text-foreground truncate">
            {displayName}
            {isCurrentUser && (
              <span className="text-muted-foreground font-normal"> (you)</span>
            )}
          </div>
          {member.username && (
            <div className="font-mono text-xs text-muted-foreground truncate">
              @{member.username}
            </div>
          )}
        </div>
      </div>

      <div className="py-3">
        {isPending ? (
          <span className="text-[13px] text-muted-foreground italic">
            Invited
          </span>
        ) : (
          <RoleCell
            role={member.role}
            isAdmin={isAdmin}
            canChange={!isCurrentUser}
            onChangeRole={onChangeRole}
          />
        )}
      </div>

      <div className="py-3">
        <span className="text-xs text-foreground">
          {formatRelativeTime(member.created_at)}
        </span>
      </div>

      <div className="flex justify-end">
        {isAdmin && !isCurrentUser && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-xs">
                <MoreHorizontal className="size-3.5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-[160px]">
              <DropdownMenuItem
                onSelect={onRemove}
                className="text-destructive focus:text-destructive focus:bg-destructive/10"
              >
                <Trash2 className="size-3.5 text-destructive" />
                {isPending ? "Revoke invitation" : "Remove member"}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </div>
  );
}

function EmptyState({ isAdmin, onInvite }: { isAdmin: boolean; onInvite: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <div className="flex justify-center mb-3 text-muted-foreground">
        <Users className="size-6" />
      </div>
      <p className="text-sm font-medium text-foreground">No members yet</p>
      <p className="text-xs text-muted-foreground mt-1 mb-4">
        {isAdmin ? "Invite people to join this organization" : "No one else has joined this organization yet"}
      </p>
      {isAdmin && (
        <Button size="sm" onClick={onInvite}>
          <UserPlus className="size-3.5" />
          Invite members
        </Button>
      )}
    </div>
  );
}

export default function OrgMembersSettings() {
  const { orgSlug = "" } = useParams();
  const { user, role } = useAuth();
  const { data: membersData, isLoading: membersLoading, error: membersError } =
    useAccountMembers(orgSlug, { includePending: true });
  const updateRole = useUpdateMemberRole();
  const removeMember = useRemoveAccountMember();
  const [inviteOpen, setInviteOpen] = useState(false);

  // Session role reflects the currently-switched org context
  const isAdmin = role === "admin" || role === "owner";
  const members = membersData?.members ?? [];

  const handleChangeRole = (member: AccountMember, newRole: string) => {
    updateRole.mutate(
      { account: orgSlug, userId: member.user_id, role: newRole },
      {
        onError: (err) => {
          console.error("Failed to update role:", err);
        },
      },
    );
  };

  const handleRemove = (member: AccountMember) => {
    removeMember.mutate({ account: orgSlug, userId: member.user_id });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-heading-2 text-foreground">Members</h2>
          <p className="text-[13px] text-muted-foreground mt-1">
            Manage who has access to this organization
          </p>
        </div>
        {isAdmin && (
          <div className="flex items-center gap-2 shrink-0">
            <Button size="sm" onClick={() => setInviteOpen(true)}>
              <UserPlus className="size-3.5" />
              Invite members
            </Button>
          </div>
        )}
      </div>

      <Separator />

      {(updateRole.isError || removeMember.isError) && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3">
          <p className="text-[13px] text-destructive">
            {((updateRole.error || removeMember.error) as Error)?.message ||
              "An error occurred. You may not have permission for this action."}
          </p>
        </div>
      )}

      {membersLoading ? (
        <div className="flex items-center gap-2 py-8 text-[13px] text-muted-foreground">
          <Loader2 size={14} className="animate-spin" />
          Loading...
        </div>
      ) : membersError ? (
        <p className="text-[13px] text-muted-foreground py-4">
          Failed to load members.
        </p>
      ) : members.length === 0 ? (
        <EmptyState isAdmin={isAdmin} onInvite={() => setInviteOpen(true)} />
      ) : (
        <div className="rounded-[10px] border border-border overflow-hidden">
          <div
            className={`grid ${GRID_COLS} gap-x-3 px-4 border-b border-border bg-muted`}
          >
            {["User", "Role", "Joined", ""].map((h, i) => (
              <div
                key={i}
                className="font-mono text-label tracking-wider text-faint-foreground py-2.5 uppercase text-left"
              >
                {h}
              </div>
            ))}
          </div>
          <div className="bg-surface">
            {members.map((member, i) => (
              <MemberRow
                key={member.user_id}
                member={member}
                isLast={i === members.length - 1}
                isCurrentUser={member.user_id === user?.id}
                isAdmin={isAdmin}
                onChangeRole={(r) => handleChangeRole(member, r)}
                onRemove={() => handleRemove(member)}
              />
            ))}
          </div>
        </div>
      )}

      <InviteMembersDialog
        orgSlug={orgSlug}
        open={inviteOpen}
        onOpenChange={setInviteOpen}
      />
    </div>
  );
}
