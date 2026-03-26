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

export interface MappedEnvVar {
  key: string;
  value: string;
  secret: boolean;
  source: string;
}

export interface MappedContainer {
  name: string;
  ready: boolean;
  vars: MappedEnvVar[];
}

export interface DomainUrl {
  name: string;
  url: string;
  type?: string;
}

export interface ServiceRow {
  id: string;
  workloadName: string;
  title: string;
  isAgentService: boolean;
  readyText: string;
  uptime: string;
  containers: MappedContainer[];
  url?: string;
  urls?: DomainUrl[];
}
