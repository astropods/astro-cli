import { useCallback, useEffect, useRef } from "react";
import { cn } from "@/lib/utils";
import { getAvatarUrl, getFallbackAvatarUrl } from "@/lib/assets";
import { useAvatarBust } from "@/lib/avatar-bust";

// Session-local cache of handles whose CDN avatar failed. Lets remounted
// components initialize to the fallback immediately without an effect.
const failedHandles = new Set<string>();

export interface UserAvatarProps {
  handle: string;
  name: string;
  className?: string;
}

export function UserAvatar({ handle, name, className }: UserAvatarProps) {
  const override = useAvatarBust(handle);
  const imgRef = useRef<HTMLImageElement>(null);
  const fallback = getFallbackAvatarUrl();
  const src = override ?? (failedHandles.has(handle) ? fallback : getAvatarUrl(handle));

  const onError = useCallback((e: React.SyntheticEvent<HTMLImageElement>) => {
    const img = e.currentTarget;
    if (img.src !== fallback) {
      failedHandles.add(handle);
      img.src = fallback;
    }
  }, [handle, fallback]);

  // Catch errors that fired before React hydrated and onError was attached.
  useEffect(() => {
    const img = imgRef.current;
    if (img && img.complete && img.naturalWidth === 0 && img.src !== fallback) {
      failedHandles.add(handle);
      img.src = fallback;
    }
  }, [handle, fallback]);

  return (
    <img
      ref={imgRef}
      src={src}
      alt={name}
      onError={onError}
      className={cn("size-8 shrink-0 rounded-full object-cover", className)}
    />
  );
}
