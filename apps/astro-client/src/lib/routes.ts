export const dashboardPath = "/agents";
export const explorePath = "/explore";
export const accountBlueprintsPath = "/blueprints";

export function blueprintsPathForAuth(isAuthenticated: boolean) {
  return isAuthenticated ? accountBlueprintsPath : explorePath;
}

export function accountProfilePath(account: string) {
  return `/${account}`;
}

// `enum` would be more idiomatic, but this project enables tsconfig's
// `erasableSyntaxOnly`, which forbids enums (they emit runtime code). A
// frozen `as const` object gives the same call-site ergonomics
// (`DeploymentTab.Monitor`) and is fully erasable.
export const DeploymentTab = {
  Monitor: "monitor",
  Traces: "traces",
  Deployment: "deployments",
  Dataset: "dataset",
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

export function deploymentTracePath(
  account: string,
  deploymentId: string,
  traceId: string,
) {
  return [
    `${deploymentPath(account, deploymentId, DeploymentTab.Traces)}?trace=${encodeURIComponent(traceId)}`,
    traceRowAnchorId(traceId),
  ].join("#");
}

export function deploymentTracesPath(account: string, deploymentId: string) {
  return deploymentPath(account, deploymentId, DeploymentTab.Traces);
}

export function traceRowAnchorId(traceId: string) {
  // `-` is the escape delimiter, so it must be escaped too — otherwise a literal
  // `-` is indistinguishable from an escape and distinct IDs can collide (e.g.
  // "a b" and "a-20-b"). Escaping it keeps every `-` in the output a delimiter,
  // making the encoding injective while staying DOM-id-safe.
  return `trace-${traceId.replace(
    /[^A-Za-z0-9_]/g,
    (char) => `-${char.charCodeAt(0).toString(16)}-`,
  )}`;
}

export function deploymentConfigurePath(account: string, deploymentId: string) {
  return deploymentPath(account, deploymentId, DeploymentTab.Configure);
}

export function blueprintsAccountPath(account: string) {
  return `/blueprints?account=${encodeURIComponent(account)}`;
}

export const chatPath = "/chat";

export function chatDeploymentPath(
  deploymentId: string,
  conversationId?: string | null,
) {
  const base = `/chat/${encodeURIComponent(deploymentId)}`;
  if (!conversationId) return base;
  return `${base}?conversation=${encodeURIComponent(conversationId)}`;
}

export const insightsPath = "/insights";
export const knowledgePath = "/knowledge";
export const newKnowledgePath = "/knowledge/new";

export function knowledgeDetailPath(name: string) {
  return `/knowledge/${name}`;
}
