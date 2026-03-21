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
  if (s === "error" || s === "failed" || s === "crashloopbackoff" || (deployment.ready === 0 && deployment.replicas > 0)) {
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
