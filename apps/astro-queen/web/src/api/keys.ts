export const adminKeys = {
  all: ["admin"] as const,
  deployments: () => [...adminKeys.all, "deployments"] as const,
  deployment: (id: string) => [...adminKeys.all, "deployment", id] as const,
  accounts: () => [...adminKeys.all, "accounts"] as const,
  blueprints: () => [...adminKeys.all, "blueprints"] as const,
  blueprintBuilds: (account: string, name: string) =>
    [...adminKeys.all, "blueprintBuilds", account, name] as const,
  clusterStatus: (ns: string) =>
    [...adminKeys.all, "clusterStatus", ns] as const,
  clusters: (enabledOnly?: boolean) =>
    [...adminKeys.all, "clusters", enabledOnly ?? false] as const,
  images: () => [...adminKeys.all, "images"] as const,
  schema: () => [...adminKeys.all, "schema"] as const,
  query: (sql: string) => [...adminKeys.all, "query", sql] as const,
  podLogs: (id: string, pod: string) =>
    [...adminKeys.all, "podLogs", id, pod] as const,
  podEnv: (id: string, pod: string) =>
    [...adminKeys.all, "podEnv", id, pod] as const,
  connectedDevices: () => [...adminKeys.all, "connectedDevices"] as const,
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
};

export const openmeterKeys = {
  all: ["openmeter"] as const,
  meters: () => [...openmeterKeys.all, "meters"] as const,
  meter: (id: string) => [...openmeterKeys.all, "meter", id] as const,
  meterQuery: (id: string) =>
    [...openmeterKeys.all, "meterQuery", id] as const,
  meterGroupBy: (id: string, key: string) =>
    [...openmeterKeys.all, "meterGroupBy", id, key] as const,
  features: () => [...openmeterKeys.all, "features"] as const,
  feature: (id: string) => [...openmeterKeys.all, "feature", id] as const,
  customers: () => [...openmeterKeys.all, "customers"] as const,
  customer: (id: string) => [...openmeterKeys.all, "customer", id] as const,
  customerAccess: (id: string) =>
    [...openmeterKeys.all, "customerAccess", id] as const,
  customerApps: (id: string) =>
    [...openmeterKeys.all, "customerApps", id] as const,
  customerEntitlements: (id: string) =>
    [...openmeterKeys.all, "customerEntitlements", id] as const,
  entitlementValue: (custId: string, entId: string) =>
    [...openmeterKeys.all, "entitlementValue", custId, entId] as const,
  entitlementGrants: (custId: string, entId: string) =>
    [...openmeterKeys.all, "entitlementGrants", custId, entId] as const,
  plans: () => [...openmeterKeys.all, "plans"] as const,
  plan: (id: string) => [...openmeterKeys.all, "plan", id] as const,
  subscriptions: () => [...openmeterKeys.all, "subscriptions"] as const,
  subscription: (id: string) => [...openmeterKeys.all, "subscription", id] as const,
  entitlements: () => [...openmeterKeys.all, "entitlements"] as const,
  grants: () => [...openmeterKeys.all, "grants"] as const,
  events: () => [...openmeterKeys.all, "events"] as const,
  openapi: () => [...openmeterKeys.all, "openapi"] as const,
};
