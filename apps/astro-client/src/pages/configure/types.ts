import type { AgentDeployment, DeploymentTemplate } from "@/lib/api";

export interface ConfigureContext {
  account: string;
  deployment: AgentDeployment;
  template: DeploymentTemplate;
  hasNewerBuildAvailable: boolean;
  currentBuildId?: string;
  latestBuildId?: string;
}
