import type { AgentDeployment } from "./api";
import type { DeployedAgentStatus } from "../components/DeployedAgentCard";
import type { StatusIndicatorVariant } from "../components/StatusIndicator";

export const deploymentStatusVariant: Record<DeployedAgentStatus, StatusIndicatorVariant> = {
  active: "success",
  inactive: "muted",
  pending: "warning",
  undeploying: "muted",
  error: "error",
};

export const deploymentStatusLabel: Record<DeployedAgentStatus, string> = {
  active: "Live",
  inactive: "Inactive",
  pending: "Deploying",
  undeploying: "Undeploying",
  error: "Error",
};

export function mapDeploymentStatus(deployment: AgentDeployment): DeployedAgentStatus {
  const s = deployment.status?.toLowerCase() ?? "";
  if (s === "undeploying") {
    return "undeploying";
  }
  if (s === "error" || s === "failed" || s === "crashloopbackoff") {
    return "error";
  }
  if (s === "pending" || s === "provisioning" || s === "deploying" || deployment.ready < deployment.replicas) {
    return "pending";
  }
  if (deployment.ready === 0 && deployment.replicas > 0) {
    return "error";
  }
  if (deployment.replicas === 0) {
    return "inactive";
  }
  return "active";
}

export function isDeployingState(deployment: AgentDeployment): boolean {
  const s = deployment.status?.toLowerCase() ?? "";
  if (s === "pending" || s === "provisioning" || s === "deploying" || s === "undeploying") return true;
  return mapDeploymentStatus(deployment) === "pending";
}

export function isLiveState(deployment: AgentDeployment): boolean {
  return mapDeploymentStatus(deployment) === "active";
}

export function isPausedState(deployment: AgentDeployment): boolean {
  const s = deployment.status?.toLowerCase() ?? "";
  return s === "scaled_down" || s === "stopped";
}

export function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function formatMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

export function formatPercent(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`;
}

export function formatDaysActive(isoString: string): string {
  const days = Math.floor((Date.now() - new Date(isoString).getTime()) / (1000 * 60 * 60 * 24));
  if (days === 0) return "< 1 day";
  if (days === 1) return "1 day";
  return `${days} days`;
}

export function formatRelativeTime(isoString: string): string {
  const diffMs = new Date(isoString).getTime() - Date.now();
  const diffSecs = Math.round(diffMs / 1000);
  const diffMins = Math.round(diffSecs / 60);
  const diffHours = Math.round(diffMins / 60);
  const diffDays = Math.round(diffHours / 24);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (Math.abs(diffSecs) < 60) return "less than a minute ago";
  if (Math.abs(diffMins) < 60) return rtf.format(diffMins, "minute");
  if (Math.abs(diffHours) < 24) return rtf.format(diffHours, "hour");
  return rtf.format(diffDays, "day");
}
