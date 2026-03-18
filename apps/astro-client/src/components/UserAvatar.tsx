import { getUserDisplayName } from "@/lib/auth";
import type { User } from "@/lib/api";
import { cn } from "@/lib/utils";
import { getPresetAvatar } from "@/lib/presetAvatars";

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

  const preset = getPresetAvatar(user.id);
  return (
    <img
      src={preset.src}
      alt={getUserDisplayName(user)}
      className={cn("size-8 shrink-0 rounded-lg object-cover", className)}
    />
  );
}
