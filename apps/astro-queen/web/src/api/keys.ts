export const adminKeys = {
  all: ["admin"] as const,
  deployments: () => [...adminKeys.all, "deployments"] as const,
  deployment: (id: string) => [...adminKeys.all, "deployment", id] as const,
  accounts: () => [...adminKeys.all, "accounts"] as const,
  account: (id: string) => [...adminKeys.all, "account", id] as const,
  accountMetronomeAliases: (id: string) =>
    [...adminKeys.all, "account", id, "metronome-aliases"] as const,
  accountBilling: (id: string) =>
    [...adminKeys.all, "account", id, "billing"] as const,
  blueprints: () => [...adminKeys.all, "blueprints"] as const,
  blueprintBuilds: (account: string, name: string) =>
    [...adminKeys.all, "blueprintBuilds", account, name] as const,
  clusterStatus: (ns: string) =>
    [...adminKeys.all, "clusterStatus", ns] as const,
  clusters: () => [...adminKeys.all, "clusters"] as const,
  clusterBlockers: (id: string) => [...adminKeys.all, "clusters", id, "blockers"] as const,
  images: () => [...adminKeys.all, "images"] as const,
  schema: () => [...adminKeys.all, "schema"] as const,
  query: (sql: string) => [...adminKeys.all, "query", sql] as const,
  podLogs: (id: string, pod: string) =>
    [...adminKeys.all, "podLogs", id, pod] as const,
  podEnv: (id: string, pod: string) =>
    [...adminKeys.all, "podEnv", id, pod] as const,
  astroOpenapi: () => [...adminKeys.all, "astroOpenapi"] as const,
  jobKinds: () => [...adminKeys.all, "jobKinds"] as const,
  jobStates: () => [...adminKeys.all, "jobStates"] as const,
  adminQueues: () => [...adminKeys.all, "adminQueues"] as const,
  jobs: (params: Record<string, string | undefined>) => [...adminKeys.all, "jobs", params] as const,
  jobsAll: () => [...adminKeys.all, "jobs"] as const,
  job: (id: number) => [...adminKeys.all, "job", id] as const,
  deploymentEvents: (id: string) => [...adminKeys.all, "deploymentEvents", id] as const,
  deploymentJobs: (id: string) => [...adminKeys.all, "deploymentJobs", id] as const,
  quotaRequests: (status?: string) => [...adminKeys.all, "quotaRequests", status ?? "all"] as const,
  feedback: () => [...adminKeys.all, "feedback"] as const,
  migrations: (mismatchesOnly?: boolean) =>
    [...adminKeys.all, "migrations", mismatchesOnly ?? false] as const,
  alerts: () => [...adminKeys.all, "alerts"] as const,
  evaluators: () => [...adminKeys.all, "evaluators"] as const,
  evaluatorDrift: (id: string) => [...adminKeys.all, "evaluators", id, "drift"] as const,
};
