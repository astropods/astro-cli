import type { ReactNode } from "react";
import type { IntegrationIconStackItem } from "@/components/IntegrationIconStack";
import type { ResolvedIntegration } from "@/lib/api";
import { getIntegrationIconUrl } from "@/lib/assets";

export function getIntegrationIcon(id: string): ReactNode {
  return (
    <img
      src={getIntegrationIconUrl(id, "light")}
      alt=""
      className="h-full w-full object-contain"
      loading="lazy"
    />
  );
}

export function getIntegrationItems(
  integrations: ResolvedIntegration[],
): IntegrationIconStackItem[] {
  return integrations
    .filter((i) => i.known)
    .map((i) => ({ name: i.name, icon: getIntegrationIcon(i.id) }));
}
