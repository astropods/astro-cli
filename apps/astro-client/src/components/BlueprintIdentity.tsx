import { AvatarImage } from "@/components/AvatarImage";
import { getAgentAvatarUrl } from "@/lib/assets";
import { useAgentAvatarBust } from "@/lib/avatar-bust";

interface BlueprintIdentityProps {
  account: string;
  name: string;
  size?: number;
  /** Server-emitted versioned blueprint avatar URL. Preferred over the
   *  deterministic fallback, but the local upload override still wins so a
   *  just-uploaded image shows instantly. */
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
    <AvatarImage
      src={bust ?? url ?? getAgentAvatarUrl(account, name)}
      alt={name}
      size={size}
      className={className}
    />
  );
}
