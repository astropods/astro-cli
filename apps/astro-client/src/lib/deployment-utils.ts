import type { AgentDeployment } from "./api";
import type { DeployedAgentStatus } from "../components/DeployedAgentCard";
import type { StatusIndicatorVariant } from "../components/StatusIndicator";

// UI lifecycle state — superset of DeployedAgentStatus, adds live-reveal transitional state
// Future sub-states: 'rolling-back' | 'paused' (require backend support)
export type UILifecycleState = 'pending' | 'live-reveal' | 'active' | 'inactive' | 'error';

export type StageStatus = 'done' | 'running' | 'pending';

export interface DeploymentStage {
  label: string;
  status: StageStatus;
}

export function deriveDeploymentStages(deployment: AgentDeployment): DeploymentStage[] {
  const pods = deployment.pods ?? [];
  const hasPods = pods.length > 0;
  const anyRunning = pods.some(p => p.phase === 'Running');
  const ready = deployment.ready ?? 0;
  const replicas = deployment.replicas ?? 1;

  const s = (label: string, status: StageStatus): DeploymentStage => ({ label, status });

  return [
    s('Configuration validated',    'done'),
    s('Image pulled',               hasPods ? 'done' : 'running'),
    s('Pod scheduled',              hasPods ? 'done' : 'pending'),
    s('Container started',          anyRunning ? 'done' : hasPods ? 'running' : 'pending'),
    s('Health checks passing',      ready > 0 ? 'done' : anyRunning ? 'running' : 'pending'),
    s('Going live',                 ready >= replicas && replicas > 0 ? 'done' : ready > 0 ? 'running' : 'pending'),
  ];
}

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
  const s = (deployment.status ?? '').toLowerCase();

  // Explicit error states from the server
  if (s === 'error' || s === 'failed') {
    return "error";
  }
  // Explicit pending/deploying — server string OR optimistic-update 'pending'
  if (s === 'pending' || s === 'creating' || s === 'deploying') {
    return "pending";
  }
  // Pods expected but not yet ready → still deploying
  if (deployment.replicas > 0 && deployment.ready < deployment.replicas) {
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
