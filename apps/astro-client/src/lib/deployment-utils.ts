import type { AgentDeployment } from "./api";
import type { DeployedAgentStatus } from "../components/DeployedAgentCard";
import type { StatusIndicatorVariant } from "../components/StatusIndicator";

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

/** Formats a date as "April 22nd, 2026 at 14:30:45" in the given timezone (defaults to browser local). */
export function formatDateLong(dateStr: string, timezone?: string): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  const tz = timezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
  const fmt = (opts: Intl.DateTimeFormatOptions) =>
    new Intl.DateTimeFormat('en-US', { timeZone: tz, ...opts }).format(date);
  const dayNum = parseInt(fmt({ day: 'numeric' }), 10);
  const v = dayNum % 100;
  const suffixes = ['th', 'st', 'nd', 'rd'];
  const suffix = suffixes[(v - 20) % 10] ?? suffixes[v] ?? 'th';
  const month = fmt({ month: 'long' });
  const year = fmt({ year: 'numeric' });
  const time = fmt({ hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
  return `${month} ${dayNum}${suffix}, ${year} at ${time}`;
}

export function formatDaysActive(isoString: string): string {
  const days = Math.floor((Date.now() - new Date(isoString).getTime()) / (1000 * 60 * 60 * 24));
  if (days === 0) return "< 1 day";
  if (days === 1) return "1 day";
  return `${days} days`;
}
