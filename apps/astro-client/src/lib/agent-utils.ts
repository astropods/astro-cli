import type { Agent, AgentCardAuthor, ResolvedIntegration } from "@/lib/api";

export function getLatestVersion(agent: Agent) {
  return agent.versions[0];
}

export function getLatestSpec(agent: Agent) {
  return getLatestVersion(agent)?.spec;
}

export function getAgentDescription(agent: Agent): string {
  const card = getLatestVersion(agent)?.agent_card;
  if (card?.description) return card.description;
  return agent.name;
}

export function getAgentCategories(agent: Agent): string[] {
  const card = getLatestVersion(agent)?.agent_card;
  if (card?.tags && card.tags.length > 0) return card.tags;
  return [];
}

export function getAgentReadme(agent: Agent): string | undefined {
  const card = getLatestVersion(agent)?.agent_card;
  return card?.body ?? getLatestVersion(agent)?.readme;
}

export function getAgentAuthors(agent: Agent): AgentCardAuthor[] {
  return getLatestVersion(agent)?.agent_card?.authors ?? [];
}

export function getAgentCapabilities(agent: Agent): string[] {
  return getLatestVersion(agent)?.agent_card?.capabilities ?? [];
}

/** Returns the resolved integrations from the agent card (already merged with spec by the server). */
export function getAgentIntegrations(agent: Agent): ResolvedIntegration[] {
  return getLatestVersion(agent)?.agent_card?.integrations ?? [];
}
