import { useState, useMemo, useCallback } from "react";
import { generateIdentity } from "identity-gen";
import { cn } from "@/lib/utils";
import { getAgentAvatarUrl } from "@/lib/assets";

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
  const [imgFailed, setImgFailed] = useState(false);

  const avatarUrl = url ?? getAgentAvatarUrl(account, name);

  const svg = useMemo(
    () => generateIdentity({ seed: `${account}/${name}`, size }),
    [account, name, size],
  );

  const onError = useCallback(() => setImgFailed(true), []);

  if (!imgFailed) {
    return (
      <img
        src={avatarUrl}
        alt={name}
        width={size}
        height={size}
        onError={onError}
        className={cn("object-cover", className)}
      />
    );
  }

  return (
    <div
      className={cn("[&>svg]:block [&>svg]:size-full", className)}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
