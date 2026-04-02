import type { AgentDeployment, DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";
import { mapDeploymentStatus } from "@/lib/deployment-utils";
import type { StatusIndicatorVariant } from "@/components/StatusIndicator";
import type { DeployHistoryStatus } from "./types";

export function formatDurationMs(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "—";
  const sec = Math.max(0, Math.round(ms / 1000));
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

export function resolveDeployedAtMs(h: ApiDeploymentHistoryRecord, live: AgentDeployment): number {
  const fromHist = new Date(h.deployed_at).getTime();
  if (h.is_current) {
    const fromLive = new Date(live.created_at).getTime();
    if (!Number.isFinite(fromHist) || Number.isNaN(fromHist)) return fromLive;
    return fromHist;
  }
  return fromHist;
}

export function deploymentHistoryDurationMs(
  h: ApiDeploymentHistoryRecord,
  idx: number,
  merged: ApiDeploymentHistoryRecord[],
  live: AgentDeployment,
  isCurrent: boolean,
): number | null {
  const start = resolveDeployedAtMs(h, live);
  if (!Number.isFinite(start) || Number.isNaN(start)) return null;
  if (isCurrent) return Date.now() - start;
  if (idx > 0) {
    const end = resolveDeployedAtMs(merged[idx - 1], live);
    if (!Number.isFinite(end) || Number.isNaN(end)) return null;
    return end - start;
  }
  return null;
}

export function deploymentHistoryUiStatus(h: ApiDeploymentHistoryRecord, live: AgentDeployment): DeployHistoryStatus {
  if (!h.is_current) return "undeployed";
  const ds = mapDeploymentStatus(live);
  if (ds === "error") return "failed";
  if (ds === "undeploying") return "undeploying";
  if (ds === "deploying") return "deploying";
  if (ds === "inactive") return "inactive";
  return "active";
}

export function statusVariant(status: DeployHistoryStatus): StatusIndicatorVariant {
  if (status === "failed") return "error";
  if (status === "undeployed") return "muted";
  if (status === "inactive") return "muted";
  if (status === "deploying") return "warning";
  if (status === "undeploying") return "muted";
  if (status === "active") return "success";
  if (status === "restarting") return "warning";
  if (status === "pausing") return "error";
  if (status === "resuming") return "success";
  return "muted";
}

export function statusLabel(status: DeployHistoryStatus): string {
  if (status === "active") return "Active";
  if (status === "inactive") return "Inactive";
  if (status === "deploying") return "Deploying";
  if (status === "undeploying") return "Undeploying";
  if (status === "failed") return "Failed";
  if (status === "restarting") return "Restarting";
  if (status === "pausing") return "Pausing";
  if (status === "resuming") return "Resuming";
  return "Undeployed";
}
