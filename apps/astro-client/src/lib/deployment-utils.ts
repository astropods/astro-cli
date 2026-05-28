import type { AgentDeployment } from "./api";
import type { StatusIndicatorVariant } from "../components/StatusIndicator";

// Canonical status enum surfaced to UI. Mapped from the AgentDeployment row
// (server-derived + DB fallbacks) by `mapDeploymentStatus` below. Status
// indicators / badges import this directly rather than reaching into a card.
export type DeployedAgentStatus =
  | "active"
  | "inactive"
  | "deploying"
  | "undeploying"
  | "error"
  | "restarting"
  | "pausing"
  | "resuming";

export const deploymentStatusVariant: Record<DeployedAgentStatus, StatusIndicatorVariant> = {
  active: "success",
  inactive: "muted",
  deploying: "warning",
  undeploying: "muted",
  error: "error",
  restarting: "warning",
  pausing: "error",
  resuming: "success",
};

export const deploymentStatusLabel: Record<DeployedAgentStatus, string> = {
  active: "Active",
  inactive: "Inactive",
  deploying: "Deploying",
  undeploying: "Undeploying",
  error: "Error",
  restarting: "Restarting",
  pausing: "Pausing",
  resuming: "Resuming",
};

/**
 * Returns true when workload container readiness hasn't caught up with the
 * deployment's desired replica count. This happens briefly after pause/resume:
 *  - After pause: replicas=0 but containers may still report ready:true
 *  - After resume: replicas>0 but containers may still report ready:false
 *
 * Scoped to Deployment/StatefulSet — Job/CronJob health lives on `wl.status`,
 * not `containers[].ready`. Their spec-seeded entries serialize as
 * `ready: false` whenever a live pod isn't matched (Idle CronJobs, finished
 * Jobs), which would otherwise trap polling in a 3s refetch loop.
 */
export function hasContainerMismatch(dep: AgentDeployment | null | undefined): boolean {
  if (!dep) return false;
  const workloads = (dep.workloads ?? []).filter(
    (wl) => wl.kind === "Deployment" || wl.kind === "StatefulSet",
  );
  if (dep.replicas === 0) {
    return workloads.some((wl) => (wl.containers ?? []).some((c) => c.ready));
  }
  return workloads.some((wl) => (wl.containers ?? []).some((c) => !c.ready));
}

export const launchUnavailableMessage =
  "Launch is unavailable while we create your custom URL";

export function getMessagingEndpoint(deployment: AgentDeployment | null | undefined) {
  return deployment?.external_urls?.find((u) => u.type === "messaging");
}

export function isLaunchReady(deployment: AgentDeployment | null | undefined): boolean {
  const messaging = getMessagingEndpoint(deployment);
  if (!messaging?.url) return false;
  if (deployment?.messaging_available === false) return false;
  // Require explicit ready: true — the API omits ready when false (Go json omitempty),
  // so treating absent/undefined as ready would keep Launch clickable while provisioning.
  return messaging.ready === true;
}

export function mapDeploymentStatus(deployment: AgentDeployment): DeployedAgentStatus {
  const s = deployment.status?.toLowerCase() ?? "";
  if (s === "undeploying") {
    return "undeploying";
  }
  if (s === "error" || s === "failed" || s === "crashloopbackoff") {
    return "error";
  }
  if (s === "pending" || s === "provisioning" || s === "deploying" || deployment.ready < deployment.replicas) {
    return "deploying";
  }
  if (deployment.ready === 0 && deployment.replicas > 0) {
    return "error";
  }
  if (deployment.replicas === 0) {
    return "inactive";
  }
  if (hasContainerMismatch(deployment)) {
    return "deploying";
  }
  return "active";
}

export function isDeployingState(deployment: AgentDeployment): boolean {
  const s = deployment.status?.toLowerCase() ?? "";
  if (s === "pending" || s === "provisioning" || s === "deploying" || s === "undeploying") return true;
  return mapDeploymentStatus(deployment) === "deploying";
}

export function isLiveState(deployment: AgentDeployment): boolean {
  return mapDeploymentStatus(deployment) === "active";
}

export function isPausedState(deployment: AgentDeployment): boolean {
  const s = deployment.status?.toLowerCase() ?? "";
  return s === "scaled_down" || s === "stopped";
}

export function formatRelativeTime(dateStr: string): string {
  const diffSecs = Math.round((new Date(dateStr).getTime() - Date.now()) / 1000);
  const diffMins = Math.round(diffSecs / 60);
  const diffHours = Math.round(diffMins / 60);
  const diffDays = Math.round(diffHours / 24);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (Math.abs(diffSecs) < 60) return "just now";
  if (Math.abs(diffMins) < 60) return rtf.format(diffMins, "minute");
  if (Math.abs(diffHours) < 24) return rtf.format(diffHours, "hour");
  return rtf.format(diffDays, "day");
}

export function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}


export function formatDaysActive(isoString: string): string {
  const days = Math.floor((Date.now() - new Date(isoString).getTime()) / (1000 * 60 * 60 * 24));
  if (days === 0) return "< 1 day";
  if (days === 1) return "1 day";
  return `${days} days`;
}
