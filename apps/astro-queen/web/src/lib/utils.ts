import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
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
