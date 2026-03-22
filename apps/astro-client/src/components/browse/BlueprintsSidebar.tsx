import { Globe } from "lucide-react";
import { BuildingOffice2Icon } from "@heroicons/react/24/outline";
import { SidebarNav, SidebarNavItem } from "@/components/ui/sidebar-layout";
import { Separator } from "@/components/ui/separator";
import { UserAvatar } from "@/components/UserAvatar";
import { useAuth, getUserDisplayName } from "@/lib/auth";
import { blueprintsPaths } from "@/lib/routes";

export function BlueprintsSidebar() {
  const { user, personalAccount, accounts, isAuthenticated } = useAuth();
  const orgs = accounts.filter((a) => a.type === "organization");

  return (
    <SidebarNav label="View">
      <SidebarNavItem to={blueprintsPaths.discover}>
        <span className="flex items-center gap-2">
          <Globe className="size-4 shrink-0" />
          Discover
        </span>
      </SidebarNavItem>

      {isAuthenticated && personalAccount && user && (
        <>
          <Separator className="my-1" />
          <SidebarNavItem to={blueprintsPaths.personal}>
            <span className="flex items-center gap-2">
              <UserAvatar
                handle={personalAccount.name}
                name={getUserDisplayName(user)}
                avatarVersion={personalAccount.avatar_version}
                className="!size-4"
              />
              Personal
            </span>
          </SidebarNavItem>
        </>
      )}

      {isAuthenticated &&
        orgs.map((org) => (
          <SidebarNavItem key={org.id} to={blueprintsPaths.account(org.name)}>
            <span className="flex items-center gap-2">
              <BuildingOffice2Icon className="size-4 shrink-0" />
              {org.name}
            </span>
          </SidebarNavItem>
        ))}
    </SidebarNav>
  );
}
