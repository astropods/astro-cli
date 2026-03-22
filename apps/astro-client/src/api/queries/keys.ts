// Centralized query key factories.
// Every key used with useQuery/invalidateQueries should originate here
// so invalidation is predictable and typo-proof.

export const accountKeys = {
  profile: ['profile'] as const,
  detail: (name: string) => ['accounts', name] as const,
  checkName: (name: string) => ['accounts', 'check', name] as const,
  search: (q: string, type?: string) => ['accounts', 'search', q, type] as const,
};

export const agentKeys = {
  all: ['agents'] as const,
  byAccount: (account: string) => ['agents', 'account', account] as const,
  detail: (account: string, name: string) => ['agents', account, name] as const,
  template: (account: string, name: string) =>
    ['agents', account, name, 'template'] as const,
  prefilledTemplate: (account: string, name: string, deploymentId: string) =>
    ['agents', account, name, 'template', deploymentId] as const,
};

export const observabilityKeys = {
  metrics: (deploymentId: string, params?: Record<string, string>) =>
    ['observability', 'metrics', deploymentId, params] as const,
  summary: (deploymentId: string, params?: Record<string, string>) =>
    ['observability', 'summary', deploymentId, params] as const,
  traces: (deploymentId: string, params?: Record<string, string>) =>
    ['observability', 'traces', deploymentId, params] as const,
};

export const usageKeys = {
  byAccount: (account: string) => ['usage', account] as const,
  quotaRequests: (account: string) => ['usage', 'quotaRequests', account] as const,
};

export const deploymentKeys = {
  all: (account: string) => ['deployments', account] as const,
  logs: (deploymentId: string, workloadName: string, container: string, timeRange?: string) =>
    ['deployments', deploymentId, 'logs', workloadName, container, timeRange] as const,
  spec: (account: string, name: string) =>
    ['deployments', account, name, 'spec'] as const,
  history: (account: string, name: string, deploymentId?: string) =>
    ['deployments', account, name, 'history', deploymentId ?? 'all'] as const,
};
