import type { Blueprint, BlueprintAuthor, BlueprintCardData, BlueprintCardRepo, ResolvedIntegration } from "@/lib/api";

export function getLatestVersion(blueprint: Blueprint) {
  return blueprint.versions[0];
}

export function getLatestSpec(blueprint: Blueprint) {
  return getLatestVersion(blueprint)?.spec;
}

/** Returns the agent card from the latest version, falling back to the draft card (from a pre-build AGENT.md scan). */
export function getEffectiveCard(blueprint: Blueprint): BlueprintCardData | undefined {
  return getLatestVersion(blueprint)?.agent_card ?? blueprint.draft_card;
}

export function getBlueprintDescription(blueprint: Blueprint): string {
  const card = getEffectiveCard(blueprint);
  if (card?.description) return card.description;
  return blueprint.name;
}

export function getBlueprintCategories(blueprint: Blueprint): string[] {
  const card = getEffectiveCard(blueprint);
  if (card?.tags && card.tags.length > 0) return card.tags;
  return [];
}

export function getBlueprintReadme(blueprint: Blueprint): string | undefined {
  const card = getEffectiveCard(blueprint);
  return card?.body ?? getLatestVersion(blueprint)?.readme;
}

export function getBlueprintAuthors(blueprint: Blueprint): BlueprintAuthor[] {
  return getEffectiveCard(blueprint)?.authors ?? [];
}

export function getBlueprintCapabilities(blueprint: Blueprint): string[] {
  return getEffectiveCard(blueprint)?.capabilities ?? [];
}

export function getBlueprintRepository(blueprint: Blueprint): BlueprintCardRepo | undefined {
  return getEffectiveCard(blueprint)?.repository;
}

/** Returns the resolved integrations from the blueprint (already merged with spec by the server). */
export function getBlueprintIntegrations(blueprint: Blueprint): ResolvedIntegration[] {
  return getEffectiveCard(blueprint)?.integrations ?? [];
}
