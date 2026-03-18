import type { ResolvedIntegration } from "@/lib/api";
import type { CardIntegration } from "astro-trading-card";

/**
 * Convert resolved integrations to CardIntegration[] (names only, no icons for now).
 */
export async function resolveCardIntegrations(
  integrations: ResolvedIntegration[],
): Promise<CardIntegration[]> {
  return integrations.map((i) => ({ name: i.name }));
}
