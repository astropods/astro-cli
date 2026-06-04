import type { AdminDeployment, ClusterStatusResponse } from "@/types/admin";

/** Matches server naming: `{agentName}-agent`. */
function agentWorkloadName(agentName: string): string {
  return `${agentName}-agent`;
}

export type DisplayDeploymentStatus = {
  value: string;
  details?: string;
  /** DB is active but cluster/agent is not ready yet (same gap the product UI shows). */
  differsFromDB: boolean;
};

/**
 * Aligns Queen with astro-client GET /deployments/:id/status for DB-active rows,
 * using cluster_status already returned by admin GetDeployment.
 */
export function deriveDisplayDeploymentStatus(
  dep: AdminDeployment,
  cs: ClusterStatusResponse | undefined,
): DisplayDeploymentStatus {
  const db = dep.status;

  if (db === "scaled_down" || db === "stopped") {
    return { value: "inactive", details: "Deployment is paused", differsFromDB: true };
  }
  if (db === "undeploying") {
    return { value: "undeploying", details: "Deployment is being torn down", differsFromDB: false };
  }
  if (db === "failed") {
    return { value: "error", details: dep.error_message, differsFromDB: true };
  }
  if (db === "pending" || db === "provisioning") {
    return { value: "deploying", details: "Pods are being provisioned", differsFromDB: false };
  }

  if (db !== "active") {
    return { value: db, differsFromDB: false };
  }

  if (!cs) {
    return { value: "active", differsFromDB: false };
  }

  const agentKey = agentWorkloadName(dep.name);
  const workloads = [...(cs.deployments ?? []), ...(cs.statefulsets ?? [])];
  const agent = workloads.find((w) => w.name === agentKey);

  if (!agent) {
    return {
      value: "deploying",
      details: "Agent workload not found in cluster",
      differsFromDB: true,
    };
  }

  const ready = agent.ready_replicas ?? 0;
  const replicas = agent.replicas ?? 0;
  if (replicas > 0 && ready < replicas) {
    return {
      value: "deploying",
      details: `${ready} of ${replicas} replicas ready`,
      differsFromDB: true,
    };
  }

  return { value: "active", differsFromDB: false };
}
