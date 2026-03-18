import type { ResolvedIntegration } from "@/lib/api";
import type { CardIntegration } from "astro-trading-card";
import { getIntegrationIconUrl } from "@/lib/assets";

/**
 * Convert resolved integrations to CardIntegration[].
 * Known integrations get an icon URL; custom ones render name-only.
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
