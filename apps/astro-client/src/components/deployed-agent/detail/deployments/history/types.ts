import type { DeploymentHistoryRecord as ApiDeploymentHistoryRecord } from "@/lib/api";

export type DeployHistoryStatus = "active" | "deploying" | "undeploying" | "ready" | "failed" | "undeployed";

export interface DeploymentHistoryTableRow {
  id: string;
  status: DeployHistoryStatus;
  build: string;
  duration: string;
  time: string;
  isCurrent: boolean;
  rowLabel: string;
  source: ApiDeploymentHistoryRecord;
}