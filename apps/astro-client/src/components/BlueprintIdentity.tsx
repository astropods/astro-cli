import { useState, useMemo, useEffect, useRef } from "react";
import { generateIdentity } from "identity-gen";
import { cn } from "@/lib/utils";
import { getAgentAvatarUrl } from "@/lib/assets";

// Session-local cache of avatar URLs that loaded successfully. Lets remounted
// components (e.g. on tab switch) skip opacity-0 immediately.
const loadedUrls = new Set<string>();

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
  const avatarUrl = url ?? getAgentAvatarUrl(account, name);
  const [imgLoaded, setImgLoaded] = useState(() => loadedUrls.has(avatarUrl));
  const imgRef = useRef<HTMLImageElement>(null);

  const svg = useMemo(
    () => generateIdentity({ seed: `${account}/${name}`, size }),
    [account, name, size],
  );

  // Catch images that loaded from cache before React hydrated and onLoad fired.
  useEffect(() => {
    if (imgRef.current?.complete && imgRef.current.naturalWidth > 0) {
      loadedUrls.add(avatarUrl);
      setImgLoaded(true);
    }
  }, [avatarUrl]);

  return (
    <div className={cn("relative overflow-hidden", className)}>
      <div
        className="absolute inset-0 [&>svg]:block [&>svg]:size-full"
        dangerouslySetInnerHTML={{ __html: svg }}
      />
      <img
        ref={imgRef}
        src={avatarUrl}
        alt={name}
        width={size}
        height={size}
        onLoad={() => { loadedUrls.add(avatarUrl); setImgLoaded(true); }}
        className={cn("relative h-full w-full object-cover", imgLoaded ? "opacity-100" : "opacity-0")}
      />
    </div>
  );
}
