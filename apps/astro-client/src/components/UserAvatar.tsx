import { useCallback, useEffect, useRef } from "react";
import { cn } from "@/lib/utils";
import { getAvatarUrl, getFallbackAvatarUrl } from "@/lib/assets";
import { useAvatarBust } from "@/lib/avatar-bust";

export interface UserAvatarProps {
  handle: string;
  name: string;
  /** Server-emitted versioned avatar URL; preferred over the handle-derived
   *  fallback so the long-lived cache stays correct after an avatar change. */
  avatarUrl?: string;
  className?: string;
}

export function UserAvatar({ handle, name, avatarUrl, className }: UserAvatarProps) {
  const override = useAvatarBust(handle);
  const imgRef = useRef<HTMLImageElement>(null);

  const onError = useCallback((e: React.SyntheticEvent<HTMLImageElement>) => {
    const img = e.currentTarget;
    const fallback = getFallbackAvatarUrl();
    // Prevent infinite loop if the fallback itself fails
    if (img.src !== fallback) {
      img.src = fallback;
    }
  }, []);

  useEffect(() => {
    const img = imgRef.current;
    if (img && img.complete && img.naturalWidth === 0) {
      const fallback = getFallbackAvatarUrl();
      if (img.src !== fallback) img.src = fallback;
    }
  }, []);

  return (
    <img
      ref={imgRef}
      src={override ?? avatarUrl ?? getAvatarUrl(handle)}
      alt={name}
      onError={onError}
      className={cn("size-8 shrink-0 rounded-full object-cover", className)}
    />
  );
}
