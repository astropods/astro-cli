import type { ServiceEndpointInfo } from "./api";

const DEFAULT_PLAYGROUND_LAUNCH_BASE_URL = "https://playground.astropods.ai";
const LOCAL_BACKEND_PORT = 8888;
const MESSAGING_TARGET_PORT = 8090;

function sanitizeName(name: string): string {
  const lower = name.toLowerCase();
  const replaced = lower.replaceAll("_", "-").replaceAll(".", "-");
  const stripped = replaced.replaceAll(/[^a-z0-9-]/g, "");
  const collapsed = stripped.replaceAll(/-+/g, "-").replaceAll(/^-+|-+$/g, "");
  return collapsed.slice(0, 63).replaceAll(/-+$/g, "");
}

export function buildMessagingServiceName(agentName: string): string {
  return sanitizeName(`${agentName}-messaging`);
}

export function buildPortForwardCommand(namespace: string, agentName: string): string {
  const serviceName = buildMessagingServiceName(agentName);
  return `kubectl port-forward -n ${namespace} svc/${serviceName} ${LOCAL_BACKEND_PORT}:${MESSAGING_TARGET_PORT}`;
}

export function buildLocalPlaygroundUrl(launchBaseUrl?: string): string {
  return buildPlaygroundLaunchUrl(`http://localhost:${LOCAL_BACKEND_PORT}`, launchBaseUrl);
}

function scoreEndpoint(endpoint: ServiceEndpointInfo): number {
  const name = endpoint.name.toLowerCase();
  const type = (endpoint.type ?? "").toLowerCase();

  if (name === "api" || type === "api") return 100;
  if (name.includes("api") || type.includes("api")) return 80;
  if (name.includes("messaging") || type.includes("messaging")) return 70;
  return 10;
}

export function selectPlaygroundBackendUrl(urls: ServiceEndpointInfo[]): string | null {
  if (urls.length === 0) return null;

  const candidates = urls.filter((url) => /^https?:\/\//i.test(url.url));
  if (candidates.length === 0) return null;

  return [...candidates].sort((a, b) => scoreEndpoint(b) - scoreEndpoint(a))[0]?.url ?? null;
}

export function buildPlaygroundLaunchUrl(backendUrl: string, launchBaseUrl?: string): string {
  const base = launchBaseUrl?.trim() || DEFAULT_PLAYGROUND_LAUNCH_BASE_URL;
  const url = new URL(base);
  url.searchParams.set("backend", backendUrl);
  return url.toString();
}

export function buildPlaygroundCommand(backendUrl: string): string {
  return `ast playground ${backendUrl}`;
}

export function isLocalEnv(): boolean {
  if (typeof window === "undefined") return false;
  const h = window.location.hostname;
  return h === "localhost" || h === "127.0.0.1" || h.endsWith(".local") || h.startsWith("local.");
}
