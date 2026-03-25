import type { Blueprint, BlueprintAuthor, ResolvedIntegration } from "@/lib/api";

export function getLatestVersion(blueprint: Blueprint) {
  return blueprint.versions[0];
}

export function getLatestSpec(blueprint: Blueprint) {
  return getLatestVersion(blueprint)?.spec;
}

export function getBlueprintDescription(blueprint: Blueprint): string {
  const card = getLatestVersion(blueprint)?.agent_card;
  if (card?.description) return card.description;
  return blueprint.name;
}

export function getBlueprintCategories(blueprint: Blueprint): string[] {
  const card = getLatestVersion(blueprint)?.agent_card;
  if (card?.tags && card.tags.length > 0) return card.tags;
  return [];
}

export function getBlueprintReadme(blueprint: Blueprint): string | undefined {
  const card = getLatestVersion(blueprint)?.agent_card;
  return card?.body ?? getLatestVersion(blueprint)?.readme;
}

export function getBlueprintAuthors(blueprint: Blueprint): BlueprintAuthor[] {
  return getLatestVersion(blueprint)?.agent_card?.authors ?? [];
}

export function getBlueprintCapabilities(blueprint: Blueprint): string[] {
  return getLatestVersion(blueprint)?.agent_card?.capabilities ?? [];
}

/** Returns the resolved integrations from the blueprint (already merged with spec by the server). */
export function getBlueprintIntegrations(blueprint: Blueprint): ResolvedIntegration[] {
  return getLatestVersion(blueprint)?.agent_card?.integrations ?? [];
}
