import type { AgentDeployment } from "@/lib/api";

export interface ConfigureContext {
  account: string;
  deployment: AgentDeployment;
  hasNewerBuildAvailable: boolean;
  currentBuildId?: string;
  latestBuildId?: string;
}
