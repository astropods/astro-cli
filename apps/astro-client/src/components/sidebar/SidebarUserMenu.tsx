import {
  ChevronUpIcon,
  ArrowRightEndOnRectangleIcon,
  ArrowLeftStartOnRectangleIcon,
} from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { getUserDisplayName, getUserInitials } from "@/lib/auth-utils";
import type { User } from "@/lib/api";

import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export interface SidebarUserMenuProps {
  user?: User | null;
  isLoading?: boolean;
  isAuthenticated?: boolean;
  onSignIn?: () => void;
  onSignOut?: () => void;
}

export function SidebarUserMenu({
  user,
  isLoading = false,
  isAuthenticated = false,
  onSignIn,
  onSignOut,
}: SidebarUserMenuProps) {
  if (isLoading) {
    return (
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton size="lg" disabled>
            <Loader2 className="size-4 animate-spin" />
            <span className="text-muted-foreground">Loading...</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    );
  }

  if (isAuthenticated && user) {
    return (
      <SidebarMenu>
        <SidebarMenuItem>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <SidebarMenuButton size="lg">
                {user.profile_picture_url ? (
                  <img
                    src={user.profile_picture_url}
                    alt={getUserDisplayName(user)}
                    className="size-8 shrink-0 rounded-full object-cover"
                  />
                ) : (
                  <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-gray-800 text-xs font-medium text-white">
                    {getUserInitials(user)}
                  </div>
                )}
                <div className="flex min-w-0 flex-col gap-0.5 leading-none">
                  <span className="truncate font-semibold">{getUserDisplayName(user)}</span>
                  <span className="truncate text-xs text-muted-foreground">{user.email}</span>
                </div>
                <ChevronUpIcon className="ml-auto size-4" />
              </SidebarMenuButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              side="top"
              className="w-[--radix-dropdown-menu-trigger-width]"
            >
              <DropdownMenuItem onClick={onSignOut}>
                <ArrowLeftStartOnRectangleIcon />
                Sign Out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </SidebarMenuItem>
      </SidebarMenu>
    );
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton onClick={onSignIn} size="lg">
          <ArrowRightEndOnRectangleIcon />
          <span className="font-semibold">Sign In</span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
