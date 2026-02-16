// Centralized query key factories.
// Every key used with useQuery/invalidateQueries should originate here
// so invalidation is predictable and typo-proof.

export const accountKeys = {
  profile: ['profile'] as const,
  detail: (name: string) => ['accounts', name] as const,
  checkName: (name: string) => ['accounts', 'check', name] as const,
};

export const agentKeys = {
  all: ['agents'] as const,
  detail: (account: string, name: string) => ['agents', account, name] as const,
  template: (account: string, name: string) =>
    ['agents', account, name, 'template'] as const,
};

export const deploymentKeys = {
  all: (account: string) => ['deployments', account] as const,
  logs: (account: string, namespace: string, pod: string, container: string, tailLines?: number) =>
    ['deployments', account, namespace, 'logs', pod, container, tailLines] as const,
  spec: (account: string, name: string) =>
    ['deployments', account, name, 'spec'] as const,
  history: (account: string, name: string) =>
    ['deployments', account, name, 'history'] as const,
  configmap: (account: string, namespace: string, cmname: string) =>
    ['deployments', account, namespace, 'configmap', cmname] as const,
  secretKeys: (account: string, namespace: string, secretName: string) =>
    ['deployments', account, namespace, 'secret', secretName, 'keys'] as const,
};
