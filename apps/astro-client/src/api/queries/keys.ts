// Centralized query key factories.
// Every key used with useQuery/invalidateQueries should originate here
// so invalidation is predictable and typo-proof.
import { blueprintListParamsKey, type BlueprintListParams } from '@/lib/blueprint-list-params';

export type AllAccountResource = 'blueprints' | 'knowledge' | 'deployments';

export const allAccountKeys = {
  resource: (resource: AllAccountResource) => ['all-accounts', resource] as const,
  list: (resource: AllAccountResource, accounts: string[]) =>
    ['all-accounts', resource, 'list', accounts] as const,
  target: (resource: AllAccountResource, purpose: 'poll' | 'retry', accounts: string[]) =>
    ['all-accounts', resource, purpose, accounts] as const,
};

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
  flows: (deploymentId: string, direction: string, window?: string, sort?: string, limit?: number) =>
    ['network', 'flows', deploymentId, direction, window, sort, limit] as const,
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
  traces: (
    deploymentId: string,
    window?: string,
    params?: Record<string, string>,
  ) => ['observability', 'traces', deploymentId, window, params] as const,
  tracesPaged: (
    deploymentId: string,
    window?: string,
    params?: Record<string, string>,
  ) => ['observability', 'traces', 'paged', deploymentId, window, params] as const,
  traceUsers: (deploymentId: string, window?: string) =>
    ['observability', 'trace-users', deploymentId, window] as const,
  traceDetail: (deploymentId: string, traceId: string) =>
    ['observability', 'trace-detail', deploymentId, traceId] as const,
  observationDetail: (deploymentId: string, observationId: string) =>
    ['observability', 'observation-detail', deploymentId, observationId] as const,
  accountSummary: (account: string, window?: string) =>
    ['observability', 'account-summary', account, window] as const,
  deploymentSummaries: (account: string) =>
    ['observability', 'deployment-summaries', account] as const,
  insights: (account: string, params?: Record<string, string | undefined>) =>
    params
      ? ['observability', 'insights', account, params] as const
      : ['observability', 'insights', account] as const,
};

export const usageKeys = {
  byAccount: (account: string) => ['usage', account] as const,
  quotaRequests: (account: string) => ['usage', 'quotaRequests', account] as const,
};

export const billingKeys = {
  usage: (account: string, from?: string, to?: string) =>
    ['billing', account, 'usage', from ?? '', to ?? ''] as const,
  invoices: (account: string) => ['billing', account, 'invoices'] as const,
  invoicePdf: (account: string, invoiceId: string) =>
    ['billing', account, 'invoices', invoiceId, 'pdf'] as const,
  balances: (account: string) => ['billing', account, 'balances'] as const,
  paymentMethod: (account: string) => ['billing', account, 'payment-method'] as const,
};

export const variableKeys = {
  byAccount: (account: string) => ['variables', account] as const,
}

export const otelIngestKeyKeys = {
  byAccount: (account: string) => ['otel-ingest-keys', account] as const,
}

export const notificationKeys = {
  preferences: (account: string) => ['notification-preferences', account] as const,
  inboxConfig: () => ['notification-inbox-config'] as const,
}

export const githubKeys = {
  status: (account: string, name: string) => ['github', account, name] as const,
  repos: (account: string, name: string) => ['github', account, name, 'repos'] as const,
  accountStatus: (account: string) => ['github', account, 'status'] as const,
  accountRepos: (account: string, q: string) => ['github', account, 'repos', q] as const,
  accountConnections: (account: string) => ['github', account, 'connections'] as const,
  accountOrgs: (account: string) => ['github', account, 'orgs', 'account-scope'] as const,
  accountBranches: (account: string, repo: string) => ['github', account, 'branches', repo] as const,
};

export const slackKeys = {
  accountStatus: (account: string) => ['slack', account, 'status'] as const,
};

export const chatKeys = {
  conversations: (deploymentId: string) =>
    ['chat', 'conversations', deploymentId] as const,
  conversation: (deploymentId: string, conversationId: string) =>
    ['chat', 'conversation', deploymentId, conversationId] as const,
  agentConfig: (deploymentId: string) =>
    ['chat', 'agent-config', deploymentId] as const,
};

export const fileKeys = {
  all: (deploymentId: string) => ['files', deploymentId] as const,
  detail: (deploymentId: string, key: string) =>
    ['files', deploymentId, key] as const,
  usage: (deploymentId: string) => ['files', deploymentId, 'usage'] as const,
};

export const deploymentKeys = {
  summary: ['deployments', 'summary'] as const,
  all: (account: string) => ['deployments', account] as const,
  detail: (id: string) => ['deployments', 'detail', id] as const,
  runtime: (id: string) => ['deployments', 'detail', id, 'runtime'] as const,
  status: (id: string) => ['deployments', 'detail', id, 'status'] as const,
  logs: (deploymentId: string, workloadName: string, container: string, timeRange?: string, timezone?: string) =>
    ['deployments', deploymentId, 'logs', workloadName, container, timeRange, timezone] as const,
  spec: (account: string, name: string) =>
    ['deployments', account, name, 'spec'] as const,
  history: (account: string, name: string, deploymentId?: string) =>
    ['deployments', account, name, 'history', deploymentId ?? 'all'] as const,
  events: (deploymentId: string) =>
    ['deployments', deploymentId, 'events'] as const,
  alerts: (deploymentId: string, workload: string) =>
    ['deployments', deploymentId, 'alerts', workload] as const,
  lastError: (deploymentId: string, workloadName: string, container: string) =>
    ['deployments', deploymentId, 'lastError', workloadName, container] as const,
  podMetrics: (deploymentId: string, pod: string, range: string) =>
    ['deployments', deploymentId, 'pods', pod, 'metrics', range] as const,
};

export const evalKeys = {
  /** Prefix that matches every eval query for a deployment. */
  all: (deploymentId: string) => ['evals', deploymentId] as const,
  summary: (deploymentId: string) =>
    ['evals', deploymentId, 'summary'] as const,
  /** Prefix that matches every paginated items query (across limit/verdict). */
  itemsAll: (deploymentId: string) =>
    ['evals', deploymentId, 'items'] as const,
  items: (deploymentId: string, limit: number, verdict?: string) =>
    ['evals', deploymentId, 'items', limit, verdict ?? 'all'] as const,
  reviewQueues: (deploymentId: string) =>
    ['evals', deploymentId, 'review-queue'] as const,
  reviewQueue: (deploymentId: string, prediction?: string) =>
    [...evalKeys.reviewQueues(deploymentId), prediction ?? 'all'] as const,
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

export const supabaseKeys = {
  status: (account: string) => ['supabase', account, 'status'] as const,
  projects: (account: string) => ['supabase', account, 'projects'] as const,
  projectHealth: (account: string, ref: string) => ['supabase', account, 'projects', ref, 'health'] as const,
};
