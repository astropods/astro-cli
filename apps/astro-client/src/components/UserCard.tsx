import { ArrowLeftStartOnRectangleIcon } from "@heroicons/react/24/outline";
import { getUserDisplayName } from "@/lib/auth";
import type { User } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

export interface UserCardProps {
  user: User;
  onSignOut: () => void;
}

export function UserCard({ user, onSignOut }: UserCardProps) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <UserAvatar user={user} />
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
        onClick={onSignOut}
      >
        <ArrowLeftStartOnRectangleIcon className="size-4" />
        Sign out
      </Button>
    </div>
  );
}
