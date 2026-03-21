import { useCallback } from "react";
import { cn } from "@/lib/utils";
import { getAvatarUrl, getFallbackAvatarUrl } from "@/lib/assets";

export interface UserAvatarProps {
  handle: string;
  name: string;
  avatarVersion?: number;
  className?: string;
}

export function UserAvatar({ handle, name, avatarVersion, className }: UserAvatarProps) {
  const onError = useCallback((e: React.SyntheticEvent<HTMLImageElement>) => {
    const img = e.currentTarget;
    const fallback = getFallbackAvatarUrl();
    // Prevent infinite loop if the fallback itself fails
    if (img.src !== fallback) {
      img.src = fallback;
    }
  }, []);

  return (
    <img
      src={getAvatarUrl(handle, avatarVersion)}
      alt={name}
      onError={onError}
      className={cn("size-8 shrink-0 rounded-full object-cover", className)}
    />
  );
}
