export const dashboardPath = "/agents";
export const explorePath = "/explore";

export function accountProfilePath(account: string) {
  return `/${account}`;
}

// `enum` would be more idiomatic, but this project enables tsconfig's
// `erasableSyntaxOnly`, which forbids enums (they emit runtime code). A
// frozen `as const` object gives the same call-site ergonomics
// (`DeploymentTab.Monitor`) and is fully erasable.
export const DeploymentTab = {
  Monitor: "monitor",
  Deployment: "deployments",
  Configure: "configure",
} as const;
export type DeploymentTab = (typeof DeploymentTab)[keyof typeof DeploymentTab];

export function deploymentPath(
  account: string,
  deploymentId: string,
  tab: DeploymentTab = DeploymentTab.Deployment,
) {
  return `/${account}/agents/${deploymentId}/${tab}`;
}

export function deploymentConfigurePath(account: string, deploymentId: string) {
  return deploymentPath(account, deploymentId, DeploymentTab.Configure);
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
