import { Link } from "react-router";
import { ArrowLeftStartOnRectangleIcon, Cog6ToothIcon } from "@heroicons/react/24/outline";
import { getUserDisplayName } from "@/lib/auth";
import type { User } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

export interface UserCardProps {
  user: User;
  handle?: string;
  avatarVersion?: number;
  onSignOut: () => void;
}

export function UserCard({ user, handle, avatarVersion, onSignOut }: UserCardProps) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <UserAvatar handle={handle ?? user.id} name={getUserDisplayName(user)} avatarVersion={avatarVersion} />
        <div className="flex min-w-0 flex-col leading-tight">
          <span className="truncate text-sm font-semibold">
            {getUserDisplayName(user)}
          </span>
          <span className="truncate text-xs text-muted-foreground">
            {user.email}
          </span>
        </div>
      </div>
      <Separator />
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start gap-2"
        asChild
      >
        <Link to="/settings">
          <Cog6ToothIcon className="size-4" />
          Settings
        </Link>
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className="w-full justify-start gap-2"
        onClick={onSignOut}
      >
        <ArrowLeftStartOnRectangleIcon className="size-4" />
        Sign out
      </Button>
    </div>
  );
}
