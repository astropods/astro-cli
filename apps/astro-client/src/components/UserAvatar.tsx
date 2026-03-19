import { cn } from "@/lib/utils";
import { getPresetAvatarUrl } from "@/lib/presetAvatars";

export interface UserAvatarProps {
  accountId: string;
  name: string;
  profilePictureUrl?: string;
  className?: string;
}

export function UserAvatar({ accountId, name, profilePictureUrl, className }: UserAvatarProps) {
  if (profilePictureUrl) {
    return (
      <img
        src={profilePictureUrl}
        alt={name}
        className={cn("size-8 shrink-0 rounded-full object-cover", className)}
      />
    );
  }

  return (
    <img
      src={getPresetAvatarUrl(accountId)}
      alt={name}
      className={cn("size-8 shrink-0 rounded-lg object-cover", className)}
    />
  );
}
