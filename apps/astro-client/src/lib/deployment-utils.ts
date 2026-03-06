import type { AgentDeployment } from "./api";
import type { DeployedAgentStatus } from "../components/DeployedAgentCard";

export function mapDeploymentStatus(deployment: AgentDeployment): DeployedAgentStatus {
  if (deployment.status === "error" || (deployment.ready === 0 && deployment.replicas > 0)) {
    return "error";
  }
  if (deployment.status === "pending" || deployment.ready < deployment.replicas) {
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
