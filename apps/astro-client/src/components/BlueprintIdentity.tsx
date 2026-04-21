import { cn } from "@/lib/utils";
import { getAgentAvatarUrl, getFallbackAvatarUrl } from "@/lib/assets";
import { useAgentAvatarBust } from "@/lib/avatar-bust";

interface BlueprintIdentityProps {
  account: string;
  name: string;
  size?: number;
  /** Override the default agent avatar URL (e.g. deployment avatar). */
  url?: string;
  className?: string;
}

export function BlueprintIdentity({
  account,
  name,
  size = 128,
  url,
  className,
}: BlueprintIdentityProps) {
  const bust = useAgentAvatarBust(account, name);
  return (
    <img
      src={url ?? bust ?? getAgentAvatarUrl(account, name)}
      alt={name}
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
