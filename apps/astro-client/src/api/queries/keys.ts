// Centralized query key factories.
// Every key used with useQuery/invalidateQueries should originate here
// so invalidation is predictable and typo-proof.

export const accountKeys = {
  profile: ['profile'] as const,
  detail: (name: string) => ['accounts', name] as const,
  checkName: (name: string) => ['accounts', 'check', name] as const,
  search: (q: string, type?: string) => ['accounts', 'search', q, type] as const,
  members: (account: string) => ['accounts', account, 'members'] as const,
  pendingMembers: (account: string) => ['accounts', account, 'members', 'include-pending'] as const,
};

export const blueprintKeys = {
  all: ['agents'] as const,
  byAccount: (account: string) => ['agents', 'account', account] as const,
  detail: (account: string, name: string) => ['agents', account, name] as const,
};

export const observabilityKeys = {
  metrics: (deploymentId: string, window?: string) =>
    ['observability', 'metrics', deploymentId, window] as const,
  summary: (deploymentId: string, window?: string) =>
    ['observability', 'summary', deploymentId, window] as const,
  traces: (deploymentId: string, window?: string) =>
    ['observability', 'traces', deploymentId, window] as const,
  accountSummary: (account: string, window?: string) =>
    ['observability', 'account-summary', account, window] as const,
};

export const usageKeys = {
  byAccount: (account: string) => ['usage', account] as const,
  quotaRequests: (account: string) => ['usage', 'quotaRequests', account] as const,
};

export const variableKeys = {
  byAccount: (account: string) => ['variables', account] as const,
}

export const githubKeys = {
  status: (account: string, name: string) => ['github', account, name] as const,
  repos: (account: string, name: string) => ['github', account, name, 'repos'] as const,
  accountStatus: (account: string) => ['github', account, 'status'] as const,
  accountRepos: (account: string, q: string) => ['github', account, 'repos', q] as const,
  accountConnections: (account: string) => ['github', account, 'connections'] as const,
  accountDirs: (account: string, repo: string, ref: string) => ['github', account, 'dirs', repo, ref] as const,
};

export const deploymentKeys = {
  all: (account: string) => ['deployments', account] as const,
  detail: (id: string) => ['deployments', 'detail', id] as const,
  logs: (deploymentId: string, workloadName: string, container: string, timeRange?: string, timezone?: string) =>
    ['deployments', deploymentId, 'logs', workloadName, container, timeRange, timezone] as const,
  spec: (account: string, name: string) =>
    ['deployments', account, name, 'spec'] as const,
  history: (account: string, name: string, deploymentId?: string) =>
    ['deployments', account, name, 'history', deploymentId ?? 'all'] as const,
  events: (deploymentId: string) =>
    ['deployments', deploymentId, 'events'] as const,
};

export const auditLogKeys = {
  byAccount: (account: string) => ['auditLog', account] as const,
  filters: (account: string) => ['auditLog', account, 'filters'] as const,
};

export const knowledgeKeys = {
  all: (account: string) => ['knowledge', account] as const,
  detail: (account: string, name: string) => ['knowledge', account, name] as const,
  credentials: (account: string, name: string) => ['knowledge', account, name, 'credentials'] as const,
  logs: (account: string, name: string, timeRange: string) => ['knowledge', account, name, 'logs', timeRange] as const,
  metrics: (account: string, name: string) => ['knowledge', account, name, 'metrics'] as const,
};
