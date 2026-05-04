import type { ReactNode } from "react";
import type { IntegrationIconStackItem } from "@/components/IntegrationIconStack";
import type { CardIntegration } from "astro-trading-card";
import type { ResolvedIntegration } from "@/lib/api";
import { getIntegrationIconUrl } from "@/lib/assets";

export function getIntegrationIcon(id: string): ReactNode {
  return (
    <>
      <img
        src={getIntegrationIconUrl(id, "light")}
        alt=""
        className="h-full w-full object-contain dark:hidden"
        loading="lazy"
      />
      <img
        src={getIntegrationIconUrl(id, "dark")}
        alt=""
        className="hidden h-full w-full object-contain dark:block"
        loading="lazy"
      />
    </>
  );
}

export function getIntegrationItems(
  integrations: ResolvedIntegration[],
): IntegrationIconStackItem[] {
  return integrations
    .filter((i) => i.known)
    .map((i) => ({ name: i.name, icon: getIntegrationIcon(i.id) }));
}

/**
 * Convert resolved integrations to CardIntegration[] for trading cards.
 * Known integrations get a dark-variant icon URL; custom ones render name-only.
 */
export function resolveCardIntegrations(
  integrations: ResolvedIntegration[],
): CardIntegration[] {
  return integrations.map((i) =>
    i.known
      ? { name: i.name, iconUrl: getIntegrationIconUrl(i.id, "dark") }
      : { name: i.name },
  );
}
