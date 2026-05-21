// Centralized query key factories.
// Every key used with useQuery/invalidateQueries should originate here
// so invalidation is predictable and typo-proof.
import { blueprintListParamsKey, type BlueprintListParams } from '@/lib/blueprint-list-params';

export const accountKeys = {
  profile: ['profile'] as const,
  detail: (name: string) => ['accounts', name] as const,
  checkName: (name: string) => ['accounts', 'check', name] as const,
  search: (q: string, type?: string) => ['accounts', 'search', q, type] as const,
  members: (account: string) => ['accounts', account, 'members'] as const,
  pendingMembers: (account: string) => ['accounts', account, 'members', 'include-pending'] as const,
  orgs: (account: string) => ['accounts', account, 'orgs'] as const,
};

const blueprintAccountPrefix = (account: string) => ['agents', 'account', account] as const;

export const blueprintKeys = {
  all: ['agents'] as const,
  list: (account: string, params?: BlueprintListParams) =>
    [...blueprintAccountPrefix(account), blueprintListParamsKey(params)] as const,
  infiniteList: (account: string, params?: BlueprintListParams) =>
    [...blueprintAccountPrefix(account), blueprintListParamsKey(params), 'infinite'] as const,
  /** Prefix for all filtered list queries for an account. */
  byAccount: blueprintAccountPrefix,
  detail: (account: string, name: string) => ['agents', account, name] as const,
};

export const heartKeys = {
  all: ['hearts'] as const,
  byAccount: (account: string) => ['hearts', account] as const,
};

// Window literal shared by the dashboard's all-time stats — keep prime and
// read sites in sync.
export const OBSERVABILITY_WINDOW_ALL_TIME = "all-time" as const;

export const networkKeys = {
  summary: (deploymentId: string, window?: string) =>
    ['network', 'summary', deploymentId, window] as const,
  flows: (deploymentId: string, direction: string, window?: string, sort?: string) =>
    ['network', 'flows', deploymentId, direction, window, sort] as const,
  timeseries: (
    deploymentId: string,
    direction: string,
    metric: string,
    window?: string,
    step?: string,
    groupBy?: string,
  ) =>
    ['network', 'timeseries', deploymentId, direction, metric, window, step, groupBy] as const,
};

export const observabilityKeys = {
  metrics: (deploymentId: string, window?: string) =>
    ['observability', 'metrics', deploymentId, window] as const,
  summary: (deploymentId: string, window?: string) =>
    ['observability', 'summary', deploymentId, window] as const,
  traces: (deploymentId: string, window?: string) =>
    ['observability', 'traces', deploymentId, window] as const,
  traceDetail: (deploymentId: string, traceId: string) =>
    ['observability', 'trace-detail', deploymentId, traceId] as const,
  accountSummary: (account: string, window?: string) =>
    ['observability', 'account-summary', account, window] as const,
  activitySummary: (account: string, from?: string, to?: string, groupBy?: string) =>
    ['observability', 'activity-summary', account, from, to, groupBy] as const,
  blueprintsSummary: (account: string, from?: string, to?: string) =>
    ['observability', 'blueprints-summary', account, from, to] as const,
  usersSummary: (account: string, from?: string, to?: string) =>
    ['observability', 'users-summary', account, from, to] as const,
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
  accountOrgs: (account: string) => ['github', account, 'orgs', 'account-scope'] as const,
};

export const slackKeys = {
  accountStatus: (account: string) => ['slack', account, 'status'] as const,
};

export const deploymentKeys = {
  summary: ['deployments', 'summary'] as const,
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
  lastError: (deploymentId: string, workloadName: string, container: string) =>
    ['deployments', deploymentId, 'lastError', workloadName, container] as const,
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
