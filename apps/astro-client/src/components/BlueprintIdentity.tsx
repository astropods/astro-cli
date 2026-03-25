import { useMemo } from "react";
import { generateIdentity } from "identity-gen";
import { cn } from "@/lib/utils";

interface BlueprintIdentityProps {
  account: string;
  name: string;
  size?: number;
  avatarUrl?: string;
  className?: string;
}

export function BlueprintIdentity({
  account,
  name,
  size = 128,
  avatarUrl,
  className,
}: BlueprintIdentityProps) {
  const svg = useMemo(
    () => generateIdentity({ seed: `${account}/${name}`, size }),
    [account, name, size],
  );

  if (avatarUrl) {
    return (
      <img
        src={avatarUrl}
        alt={name}
        width={size}
        height={size}
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
