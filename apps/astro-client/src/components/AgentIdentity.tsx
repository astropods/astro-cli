import { useMemo } from "react";
import { generateIdentity } from "@saswatds/astro-identity-gen";

interface AgentIdentityProps {
  account: string;
  name: string;
  size?: number;
  className?: string;
}

export function AgentIdentity({
  account,
  name,
  size = 128,
  className,
}: AgentIdentityProps) {
  const svg = useMemo(
    () => generateIdentity({ seed: `${account}/${name}`, size }),
    [account, name, size],
  );

  return (
    <div
      className={className}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
