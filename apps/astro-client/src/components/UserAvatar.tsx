import { getUserDisplayName, getUserInitials } from "@/lib/auth";
import type { User } from "@/lib/api";
import { cn } from "@/lib/utils";

export interface UserAvatarProps {
  user: User;
  className?: string;
}

export function UserAvatar({ user, className }: UserAvatarProps) {
  if (user.profile_picture_url) {
    return (
      <img
        src={user.profile_picture_url}
        alt={getUserDisplayName(user)}
        className={cn("size-8 shrink-0 rounded-full object-cover", className)}
      />
    );
  }
  return (
    <div
      className={cn(
        "flex size-8 shrink-0 items-center justify-center rounded-full bg-teal-800 text-sm font-medium text-white",
        className,
      )}
    >
      {getUserInitials(user)}
    </div>
  );
}
