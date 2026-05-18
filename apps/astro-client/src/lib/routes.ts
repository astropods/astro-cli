export const dashboardPath = "/agents";
export const explorePath = "/explore";

export function accountProfilePath(account: string) {
  return `/${account}`;
}

export function deploymentPath(account: string, deploymentId: string) {
  return `/${account}/agents/${deploymentId}`;
}

export function deploymentConfigurePath(account: string, deploymentId: string) {
  return `${deploymentPath(account, deploymentId)}/configure`;
}

export function blueprintsAccountPath(account: string) {
  return `/blueprints?account=${encodeURIComponent(account)}`;
}

export const insightsPath = "/insights";
export const knowledgePath = "/knowledge";
export const newKnowledgePath = "/knowledge/new";

export function knowledgeDetailPath(name: string) {
  return `/knowledge/${name}`;
}
