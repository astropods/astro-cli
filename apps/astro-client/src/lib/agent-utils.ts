import type { Agent } from "@/lib/api";

export function getLatestVersion(agent: Agent) {
  return agent.versions[0];
}

export function getLatestSpec(agent: Agent) {
  return getLatestVersion(agent)?.spec;
}

export function getAgentDescription(agent: Agent): string {
  return getLatestSpec(agent)?.meta?.description ?? agent.name;
}

export function getAgentIntegrations(agent: Agent): string[] {
  const integrations = getLatestSpec(agent)?.integrations;
  if (!integrations) return [];
  return [...new Set(Object.values(integrations).map((i) => i.provider))];
}

export function getAgentCategories(agent: Agent): string[] {
  return getLatestSpec(agent)?.meta?.tags ?? [];
}
