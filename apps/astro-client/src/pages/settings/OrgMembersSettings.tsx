import { useState } from "react";
import { useParams, type MetaFunction } from "react-router";
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
import { SectionHeader } from "@/components/settings/SettingsShared";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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

export const meta: MetaFunction = () => [{ title: "Members - Organization Settings | Astro" }];

const ROLES = [
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
];

function RoleCell({
  role,
  canManage,
  disabled,
  onChangeRole,
}: {
  role: string;
  canManage: boolean;
  disabled?: boolean;
  onChangeRole: (role: string) => void;
}) {
  const label = ROLES.find((r) => r.value === role)?.label ?? role;

  if (!canManage) {
    return (
      <span className="text-[13px] text-foreground capitalize">{label}</span>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          disabled={disabled}
          className="inline-flex items-center gap-1 text-[13px] text-foreground capitalize cursor-pointer hover:text-muted-foreground transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
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
  isCurrentUser,
  canManage,
  disabled,
  onChangeRole,
  onRemove,
}: {
  member: AccountMember;
  isCurrentUser: boolean;
  canManage: boolean;
  disabled?: boolean;
  onChangeRole: (role: string) => void;
  onRemove: () => void;
}) {
  const displayName = member.display_name || member.username || member.user_id;
  const isPending = member.status === "pending";

  return (
    <TableRow>
      <TableCell>
        <div className="flex items-center gap-3 min-w-0 overflow-hidden">
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
      </TableCell>

      <TableCell>
        {isPending ? (
          <span className="text-[13px] text-muted-foreground italic">
            Invited
          </span>
        ) : (
          <RoleCell
            role={member.role}
            canManage={canManage}
            disabled={disabled}
            onChangeRole={onChangeRole}
          />
        )}
      </TableCell>

      <TableCell>
        <span className="text-body-sm text-foreground">
          {formatRelativeTime(member.created_at)}
        </span>
      </TableCell>

      <TableCell className="w-10 text-right">
        {canManage && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-xs" disabled={disabled}>
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
      </TableCell>
    </TableRow>
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
  const isOwner = role === "owner";
  const isAdmin = role === "admin" || isOwner;
  const members = membersData?.members ?? [];

  // A member can be managed by the caller when the caller has admin/owner
  // privileges AND the target isn't the caller themselves AND — to respect
  // the role hierarchy — the target isn't an owner unless the caller is too.
  // This mirrors the backend check in org.ErrOwnerManagementForbidden.
  const canManageMember = (m: AccountMember) =>
    isAdmin &&
    m.user_id !== user?.id &&
    !(m.role === "owner" && !isOwner);

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
    <>
      <SectionHeader
        title="Members"
        subtitle="Manage who has access to this organization"
        action={
          isAdmin && (
            <Button size="sm" onClick={() => setInviteOpen(true)}>
              <UserPlus className="size-3.5" />
              Invite members
            </Button>
          )
        }
      />

      <div className="space-y-6">
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
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>User</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Joined</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.map((member) => (
                <MemberRow
                  key={member.user_id}
                  member={member}
                  isCurrentUser={member.user_id === user?.id}
                  canManage={canManageMember(member)}
                  disabled={updateRole.isPending || removeMember.isPending}
                  onChangeRole={(r) => handleChangeRole(member, r)}
                  onRemove={() => handleRemove(member)}
                />
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      <InviteMembersDialog
        orgSlug={orgSlug}
        open={inviteOpen}
        onOpenChange={setInviteOpen}
      />
    </>
  );
}
