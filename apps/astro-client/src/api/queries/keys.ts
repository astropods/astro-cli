// Centralized query key factories.
// Every key used with useQuery/invalidateQueries should originate here
// so invalidation is predictable and typo-proof.

export const agentKeys = {
  all: ['agents'] as const,
  detail: (name: string) => ['agents', name] as const,
  version: (name: string, version: string) =>
    ['agents', name, version] as const,
  config: (name: string, version: string) =>
    ['agents', name, version, 'config'] as const,
};

export const deploymentKeys = {
  all: ['deployments'] as const,
  logs: (name: string, version: string, pod: string, container: string, tailLines?: number) =>
    ['deployments', name, version, 'logs', pod, container, tailLines] as const,
};
