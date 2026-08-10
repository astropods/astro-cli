import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function mutationErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return "Request failed";
}

export function formatDate(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function formatDateTime(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

export function truncateUUID(uuid: string): string {
  if (!uuid) return "-";
  if (uuid.length > 12 && uuid.includes("-")) {
    return uuid.slice(0, 8) + "\u2026";
  }
  return uuid;
}

/** Display label for cluster routing/placement ids (empty and "primary" → primary). */
export function formatClusterId(id: string | undefined): string {
  if (!id || id === "primary") return "primary";
  return id;
}

/** AWS ECR region segment from a container image reference, if present. */
export function ecrRegionFromImage(image: string): string | null {
  const match = image.match(/\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com\//i);
  return match ? match[1] : null;
}

/** AWS region from a Kubernetes node name (e.g. ip-…​.eu-west-1.compute.internal). */
export function k8sNodeRegionFromName(nodeName: string): string | null {
  if (!nodeName) return null;
  const match = nodeName.match(/\.([a-z]{2}-[a-z]+-\d+)\.compute\.internal$/i);
  return match ? match[1] : null;
}

export type LivePodPlacement = {
  podName: string;
  nodeName: string;
  nodeRegion: string | null;
  phase: string;
};

/** Summarize running/scheduling pods for live compute placement display. */
export function livePodPlacements(
  pods: { name: string; node_name: string; phase: string }[] | undefined,
): LivePodPlacement[] {
  if (!pods?.length) return [];
  return pods
    .filter((p) => p.node_name)
    .map((p) => ({
      podName: p.name,
      nodeName: p.node_name,
      nodeRegion: k8sNodeRegionFromName(p.node_name),
      phase: p.phase,
    }));
}

/** URL/query value for filtering deployments routed to the primary cluster. */
export const PRIMARY_CLUSTER_FILTER = "__primary__";

export function deploymentMatchesClusterFilter(
  deploymentClusterId: string | undefined,
  filter: string,
): boolean {
  if (filter === "all") return true;
  const routed = formatClusterId(deploymentClusterId);
  if (filter === PRIMARY_CLUSTER_FILTER) return routed === "primary";
  return routed === filter;
}

export function countDeploymentsByRoutedCluster(
  deployments: { status: string; cluster_id?: string }[],
): Map<string, number> {
  const counts = new Map<string, number>();
  for (const d of deployments) {
    if (d.status === "undeployed") continue;
    const key = formatClusterId(d.cluster_id);
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return counts;
}

export function specTargetClusterId(specJson: string | undefined): string | null {
  if (!specJson) return null;
  try {
    const parsed = JSON.parse(specJson) as { target?: { cluster_id?: string } };
    const id = parsed.target?.cluster_id;
    if (id === undefined) return null;
    return formatClusterId(id);
  } catch {
    return null;
  }
}
