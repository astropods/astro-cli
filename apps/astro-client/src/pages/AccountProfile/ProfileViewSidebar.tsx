import { useState } from "react";
import { Link } from "react-router";
import type { AccountPublic, AccountOrg, AccountMember } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { EarlyAdopterBadge } from "@/components/account-profile/EarlyAdopterBadge";
import { ProfileSidebarShell } from "./ProfileSidebarShell";
import { BotIcon, LayersIcon } from "lucide-react";

interface ProfileViewSidebarProps {
  data: AccountPublic;
  variant?: "personal" | "org";
  isAdmin: boolean;
  isInternalView?: boolean;
  blueprintCount: number;
  deploymentCount: number;
  isBlueprintsLoading?: boolean;
  isDeploymentsLoading?: boolean;
  orgs?: AccountOrg[];
  members?: AccountMember[];
  onEditOpen?: () => void;
}

export function ProfileViewSidebar({
  data,
  variant = "personal",
  isAdmin,
  isInternalView = false,
  blueprintCount,
  deploymentCount,
  isBlueprintsLoading,
  isDeploymentsLoading,
  orgs = [],
  members = [],
  onEditOpen,
}: ProfileViewSidebarProps) {
  const isOrg = variant === "org";
  const [membersOpen, setMembersOpen] = useState(false);

  const activeMembers = members.filter((m) => m.status === "active" && !!m.username);

  const stats = [
    { label: "Blueprints", value: blueprintCount, loading: isBlueprintsLoading, icon: <LayersIcon className="size-3.5" /> },
    ...(isInternalView
      ? [{ label: "Agents", value: deploymentCount, loading: isDeploymentsLoading, icon: <BotIcon className="size-3.5" /> }]
      : []),
  ];

  const badge =
    !isOrg && data.account_number != null && data.account_number <= 1000 ? (
      <Tooltip>
        <TooltipTrigger asChild>
          <EarlyAdopterBadge accountNumber={data.account_number} />
        </TooltipTrigger>
        <TooltipContent>One of the first 1,000 users on Astro</TooltipContent>
      </Tooltip>
    ) : undefined;

  return (
    <>
      <ProfileSidebarShell
        data={data}
        isAdmin={isAdmin}
        onEditOpen={onEditOpen}
        dateLabel={isOrg ? "Founded" : "Joined"}
        stats={stats}
        badge={badge}
        pronouns={isOrg ? undefined : data.pronouns}
        email={isOrg ? undefined : data.email}
      >
        {isOrg && isInternalView && activeMembers.length > 0 && (
          <>
            <div className="h-px bg-border" />
            <div className="flex flex-col gap-3">
              <div className="flex items-center justify-between">
                <p className="text-label uppercase text-muted-foreground">Members</p>
                {activeMembers.length > 12 && (
                  <Button variant="link" size="sm" className="h-auto p-0" onClick={() => setMembersOpen(true)}>
                    View all {activeMembers.length}
                  </Button>
                )}
              </div>
              <div className="flex flex-wrap gap-1.5">
                {activeMembers.slice(0, 12).map((member) => (
                  <Tooltip key={member.user_id}>
                    <TooltipTrigger asChild>
                      <Link to={`/${member.username}`}>
                        <UserAvatar
                          handle={member.username}
                          name={member.display_name || member.username}
                          className="size-8 transition-opacity hover:opacity-80"
                        />
                      </Link>
                    </TooltipTrigger>
                    <TooltipContent>{member.display_name || member.username}</TooltipContent>
                  </Tooltip>
                ))}
              </div>
            </div>
          </>
        )}

        {!isOrg && orgs.length > 0 && (
          <>
            <div className="h-px bg-border" />
            <div className="flex flex-col gap-3">
              <p className="text-label uppercase text-muted-foreground">Organizations</p>
              <div className="flex flex-wrap gap-2">
                {orgs.map((org) => (
                  <Tooltip key={org.name}>
                    <TooltipTrigger asChild>
                      <Link to={`/${org.name}`}>
                        <UserAvatar
                          handle={org.name}
                          name={org.display_name || org.name}
                          className="size-9 rounded-[6px] transition-opacity hover:opacity-80"
                        />
                      </Link>
                    </TooltipTrigger>
                    <TooltipContent>{org.display_name || org.name}</TooltipContent>
                  </Tooltip>
                ))}
              </div>
            </div>
          </>
        )}
      </ProfileSidebarShell>

      {isOrg && (
        <Dialog open={membersOpen} onOpenChange={setMembersOpen}>
          <DialogContent className="max-w-sm">
            <DialogHeader>
              <DialogTitle>Members · {activeMembers.length}</DialogTitle>
            </DialogHeader>
            <div className="flex flex-col gap-0.5 max-h-96 overflow-y-auto -mx-1">
              {activeMembers.map((member) => (
                <Link
                  key={member.user_id}
                  to={`/${member.username}`}
                  onClick={() => setMembersOpen(false)}
                  className="flex items-center gap-3 rounded-md px-3 py-2 hover:bg-muted transition-colors"
                >
                  <UserAvatar
                    handle={member.username}
                    name={member.display_name || member.username}
                    className="size-8 shrink-0"
                  />
                  <div className="min-w-0">
                    {member.display_name && (
                      <p className="text-body-sm font-medium text-foreground truncate">{member.display_name}</p>
                    )}
                    <p className="text-[11px] text-muted-foreground font-mono truncate">@{member.username}</p>
                  </div>
                  {member.role === "admin" && (
                    <span className="ml-auto text-label text-muted-foreground shrink-0">Admin</span>
                  )}
                </Link>
              ))}
            </div>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
