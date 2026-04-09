import { useCallback } from "react";
import { cn } from "@/lib/utils";
import { getAvatarUrl, getFallbackAvatarUrl } from "@/lib/assets";
import { useAvatarBust } from "@/lib/avatar-bust";

export interface UserAvatarProps {
  handle: string;
  name: string;
  className?: string;
}

export function UserAvatar({ handle, name, className }: UserAvatarProps) {
  const override = useAvatarBust(handle);

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
      src={override ?? getAvatarUrl(handle)}
      alt={name}
      onError={onError}
      className={cn("size-8 shrink-0 rounded-full object-cover", className)}
    />
  );
}
