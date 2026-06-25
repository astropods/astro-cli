import { cn } from "@/lib/utils";
import { getFallbackAvatarUrl } from "@/lib/assets";

export interface AvatarImageProps {
  src: string;
  alt: string;
  size?: number;
  className?: string;
}

/**
 * Presentational avatar `<img>` with a shared placeholder fallback. Callers
 * (BlueprintIdentity, DeploymentAvatar) resolve the source — including the
 * upload override — and pass the final URL here.
 */
export function AvatarImage({ src, alt, size = 128, className }: AvatarImageProps) {
  return (
    <img
      src={src}
      alt={alt}
      width={size}
      height={size}
      decoding="async"
      onError={(e) => {
        const fallback = getFallbackAvatarUrl();
        if (e.currentTarget.src !== fallback) e.currentTarget.src = fallback;
      }}
      className={cn("object-cover", className)}
    />
  );
}
