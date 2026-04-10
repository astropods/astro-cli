import { useState, useMemo, useCallback, useEffect, useRef } from "react";
import { generateIdentity } from "identity-gen";
import { cn } from "@/lib/utils";
import { getAgentAvatarUrl } from "@/lib/assets";
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
  const [imgFailed, setImgFailed] = useState(false);
  const imgRef = useRef<HTMLImageElement>(null);
  const bust = useAgentAvatarBust(account, name);

  const avatarUrl = url ?? bust ?? getAgentAvatarUrl(account, name);
  const showFallback = imgFailed && !bust;

  const svg = useMemo(
    () => generateIdentity({ seed: `${account}/${name}`, size }),
    [account, name, size],
  );

  const onError = useCallback(() => setImgFailed(true), []);

  useEffect(() => {
    if (imgRef.current?.complete && imgRef.current.naturalWidth === 0) {
      setImgFailed(true);
    }
  }, []);

  if (!showFallback) {
    return (
      <img
        ref={imgRef}
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
