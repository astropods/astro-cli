import type { AgentDeployment } from "./api";
import type { DeployedAgentStatus } from "../components/DeployedAgentCard";
import type { StatusIndicatorVariant } from "../components/StatusIndicator";

export const deploymentStatusVariant: Record<DeployedAgentStatus, StatusIndicatorVariant> = {
  active: "success",
  inactive: "muted",
  pending: "pending",
  error: "error",
};

export const deploymentStatusLabel: Record<DeployedAgentStatus, string> = {
  active: "Live",
  inactive: "Inactive",
  pending: "Deploying",
  error: "Error",
};

export function mapDeploymentStatus(deployment: AgentDeployment): DeployedAgentStatus {
  const s = deployment.status?.toLowerCase() ?? "";
  if (s === "error" || s === "failed" || s === "crashloopbackoff" || (deployment.ready === 0 && deployment.replicas > 0)) {
    return "error";
  }
  if (s === "pending" || s === "deploying" || deployment.ready < deployment.replicas) {
    return "pending";
  }
  if (deployment.replicas === 0) {
    return "inactive";
  }
  return "active";
}

export function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}
