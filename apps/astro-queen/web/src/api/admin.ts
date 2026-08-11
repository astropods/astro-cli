import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { deriveDisplayDeploymentStatus } from "@/lib/display-deployment-status";
import { adminKeys } from "./keys";
import type {
  ListDeploymentsResponse,
  GetDeploymentResponse,
  ListAccountsResponse,
  GetAccountResponse,
  MetronomeAliasStatus,
  RegisterAccountMetronomeResponse,
  GetAccountBillingDetailResponse,
  RetryBillingProvisionResponse,
  ForceBillingResumeResponse,
  RecoverAccountLangfuseResponse,
  RecoverAccountBifrostResponse,
  ListBlueprintsResponse,
  GetBlueprintBuildsResponse,
  ClusterStatusResponse,
  GetPodLogsResponse,
  GetPodEnvResponse,
  ListConnectedDevicesResponse,
  SendCommandResponse,
  GetDeploymentEventsResponse,
  GetDeploymentJobsResponse,
  ReapplyDeploymentResponse,
  ListClustersResponse,
  RegisterClusterRequest,
  RegisterClusterResponse,
  EnableClusterResponse,
  DisableClusterResponse,
  UpdateClusterRequest,
  UpdateClusterResponse,
  CheckClusterHealthResponse,
  GetClusterBlockersResponse,
  SetAccountClusterResponse,
  MigrateAccountDeploymentsResponse,
  InvalidateCachesResponse,
  RefreshMessagingCacheResponse,
  ListClusterMigrationsResponse,
  ListAlertsResponse,
} from "@/types/admin";

export function useEnv() {
  return useQuery({
    queryKey: ["env"],
    queryFn: () => api.get<{ env: string }>("/api/env"),
    staleTime: Infinity,
  });
}

export function useDeployments() {
  return useQuery({
    queryKey: adminKeys.deployments(),
    queryFn: () => api.get<ListDeploymentsResponse>("/api/admin/deployments"),
    refetchInterval: 5_000,
  });
}

export function useDeployment(id: string) {
  return useQuery({
    queryKey: adminKeys.deployment(id),
    queryFn: () =>
      api.get<GetDeploymentResponse>(
        `/api/admin/deployments/${encodeURIComponent(id)}`
      ),
    enabled: !!id,
    refetchInterval: (query) => {
      const dep = query.state.data?.deployment;
      if (!dep) return 5_000;
      const display = deriveDisplayDeploymentStatus(dep, query.state.data?.cluster_status);
      if (
        display.value === "deploying" ||
        display.value === "undeploying" ||
        dep.status === "pending" ||
        dep.status === "provisioning"
      ) {
        return 3_000;
      }
      return 5_000;
    },
  });
}

export function useDeleteDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/api/admin/deployments/${encodeURIComponent(id)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useRestartPod() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, pod }: { id: string; pod: string }) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(id)}/restart`, { pod }),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: adminKeys.deployment(id) });
    },
  });
}

export function useAccounts() {
  return useQuery({
    queryKey: adminKeys.accounts(),
    queryFn: () => api.get<ListAccountsResponse>("/api/admin/accounts"),
  });
}

export function useAccount(id: string) {
  return useQuery({
    queryKey: adminKeys.account(id),
    queryFn: () =>
      api.get<GetAccountResponse>(`/api/admin/accounts/${encodeURIComponent(id)}`),
    enabled: !!id,
  });
}

// Live check against Metronome — enabled only when the account has a hosted
// billing customer, since the check is a no-op otherwise.
export function useAccountMetronomeAliases(id: string, enabled: boolean) {
  return useQuery({
    queryKey: adminKeys.accountMetronomeAliases(id),
    queryFn: () =>
      api.get<MetronomeAliasStatus>(
        `/api/admin/accounts/${encodeURIComponent(id)}/metronome-aliases`,
      ),
    enabled: !!id && enabled,
  });
}

// Writes the expected ingest aliases onto the Metronome customer and returns the
// re-checked status.
export function useRecoverAccountMetronomeAliases() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<MetronomeAliasStatus>(
        `/api/admin/accounts/${encodeURIComponent(id)}/metronome-aliases/recover`,
        {},
      ),
    onSuccess: (data, id) => {
      qc.setQueryData(adminKeys.accountMetronomeAliases(id), data);
    },
  });
}

// Creates a Metronome customer for the account when it has none (idempotent).
export function useRegisterAccountMetronome() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<RegisterAccountMetronomeResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/metronome/register`,
        {},
      ),
    onSuccess: (data, id) => {
      qc.setQueryData<GetAccountResponse>(adminKeys.account(id), (old) =>
        old && old.billing
          ? { ...old, billing: { ...old.billing, metronome_customer_id: data.metronome_customer_id ?? "" } }
          : old,
      );
      qc.invalidateQueries({ queryKey: adminKeys.account(id) });
      // A fresh customer is seeded with the ingest aliases — re-check them.
      qc.invalidateQueries({ queryKey: adminKeys.accountMetronomeAliases(id) });
    },
  });
}

// Merged billing view: our status and provisioning record, plus the live
// contract verdict from Metronome. Separate from useAccount so the account page
// does not wait on a vendor API to render.
export function useAccountBilling(id: string) {
  return useQuery({
    queryKey: adminKeys.accountBilling(id),
    queryFn: () =>
      api.get<GetAccountBillingDetailResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/billing`,
      ),
    enabled: !!id,
  });
}

// Re-enqueues provisioning. force re-runs it for an already-provisioned
// account, which is how a stale credits_exhausted latch gets cleared.
export function useRetryBillingProvision() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, force }: { id: string; force?: boolean }) =>
      api.post<RetryBillingProvisionResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/billing/retry-provision${force ? "?force=true" : ""}`,
        {},
      ),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: adminKeys.accountBilling(id) });
    },
  });
}

// Restores deployments that billing suspended. Does not change billing status,
// so an account that still recomputes to suspended can be stopped again.
export function useForceBillingResume() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<ForceBillingResumeResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/billing/force-resume`,
        {},
      ),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: adminKeys.accountBilling(id) });
    },
  });
}

// Provisions the account's Langfuse project if missing (idempotent).
export function useRecoverAccountLangfuse() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<RecoverAccountLangfuseResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/langfuse/recover`,
        {},
      ),
    onSuccess: (data, id) => {
      // Reflect the new project id immediately, then reconcile.
      qc.setQueryData<GetAccountResponse>(adminKeys.account(id), (old) =>
        old
          ? {
              ...old,
              langfuse_project_id: data.langfuse_project_id,
              account: { ...old.account, has_langfuse: true },
            }
          : old,
      );
      qc.invalidateQueries({ queryKey: adminKeys.account(id) });
    },
  });
}

// Ensures the account's Bifrost customer exists (idempotent).
export function useRecoverAccountBifrost() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<RecoverAccountBifrostResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/bifrost/recover`,
        {},
      ),
    onSuccess: (data, id) => {
      qc.setQueryData<GetAccountResponse>(adminKeys.account(id), (old) =>
        old && old.billing
          ? { ...old, billing: { ...old.billing, bifrost_customer_id: data.bifrost_customer_id ?? "" } }
          : old,
      );
      qc.invalidateQueries({ queryKey: adminKeys.account(id) });
      // A new Bifrost customer becomes a Metronome alias — re-check that too.
      qc.invalidateQueries({ queryKey: adminKeys.accountMetronomeAliases(id) });
    },
  });
}

export function useRenameAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, newName }: { id: string; newName: string }) =>
      api.put(`/api/admin/accounts/${encodeURIComponent(id)}/rename`, {
        new_name: newName,
      }),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: adminKeys.accounts() });
      qc.invalidateQueries({ queryKey: adminKeys.account(id) });
    },
  });
}

export function useSetAccountCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, clusterId }: { id: string; clusterId: string }) =>
      api.put<SetAccountClusterResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/cluster`,
        { cluster_id: clusterId },
      ),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: adminKeys.accounts() });
      qc.invalidateQueries({ queryKey: adminKeys.account(id) });
    },
  });
}

/** Enqueues migration jobs for an account's deployments not yet on its
 * current cluster. Independent of useSetAccountCluster — call whenever
 * ready, not just right after a cluster change. */
export function useMigrateAccountDeployments() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<MigrateAccountDeploymentsResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/migrate-cluster`,
      ),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: adminKeys.accounts() });
      qc.invalidateQueries({ queryKey: adminKeys.account(id) });
      qc.invalidateQueries({ queryKey: adminKeys.migrations() });
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useClusterMigrations(mismatchesOnly: boolean) {
  return useQuery({
    queryKey: adminKeys.migrations(mismatchesOnly),
    queryFn: () =>
      api.get<ListClusterMigrationsResponse>(
        `/api/admin/migrations${mismatchesOnly ? "?mismatches_only=1" : ""}`,
      ),
    retry: false,
  });
}

// Manual escape hatch — busts the per-account agents-page caches (deploy
// envelope + per-deployment obs summary) without waiting for SafetyTTL.
export function useInvalidateAccountCaches() {
  return useMutation({
    mutationFn: (id: string) =>
      api.post<InvalidateCachesResponse>(
        `/api/admin/accounts/${encodeURIComponent(id)}/invalidate-cache`,
        {},
      ),
  });
}

// Failsafe — busts the agents-page caches across every account. Expensive
// at large scale; intended for incident response, not routine use.
export function useInvalidateAllCaches() {
  return useMutation({
    mutationFn: () =>
      api.post<InvalidateCachesResponse>("/api/admin/invalidate-cache", {}),
  });
}

// Evicts the messaging sidecar's ECR Docker Hub pull-through cache tag so the
// next agent pull re-imports it from Docker Hub, bypassing AWS's ~24h
// upstream-check window. Agents pick up the new sidecar on their next restart.
export function useRefreshMessagingCache() {
  return useMutation({
    mutationFn: () =>
      api.post<RefreshMessagingCacheResponse>(
        "/api/admin/refresh-messaging-cache",
        {},
      ),
  });
}

export function useBlueprints() {
  return useQuery({
    queryKey: adminKeys.blueprints(),
    queryFn: () => api.get<ListBlueprintsResponse>("/api/admin/agents"),
  });
}

export function useBlueprintBuilds(account: string, name: string) {
  return useQuery({
    queryKey: adminKeys.blueprintBuilds(account, name),
    queryFn: () =>
      api.get<GetBlueprintBuildsResponse>(
        `/api/admin/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}/builds`
      ),
    enabled: !!account && !!name,
  });
}

export function useClusterStatus(namespace: string) {
  return useQuery({
    queryKey: adminKeys.clusterStatus(namespace),
    queryFn: () =>
      api.get<ClusterStatusResponse>(
        `/api/admin/cluster-status?namespace=${encodeURIComponent(namespace)}`
      ),
    enabled: !!namespace,
  });
}

export function usePodLogs(id: string, pod: string, container?: string) {
  return useQuery({
    queryKey: [...adminKeys.podLogs(id, pod), container ?? ""],
    queryFn: () =>
      api.get<GetPodLogsResponse>(
        `/api/admin/pods/${encodeURIComponent(id)}/${encodeURIComponent(pod)}/logs${container ? `?container=${encodeURIComponent(container)}` : ""}`
      ),
    enabled: !!id && !!pod,
  });
}

export function useConnectedDevices() {
  return useQuery({
    queryKey: adminKeys.connectedDevices(),
    queryFn: () =>
      api.get<ListConnectedDevicesResponse>("/api/admin/devices"),
    refetchInterval: 30_000,
  });
}

export function useSendCommand() {
  return useMutation({
    mutationFn: ({
      deviceId,
      command,
      timeoutSeconds,
    }: {
      deviceId: string;
      command: string;
      timeoutSeconds?: number;
    }) =>
      api.post<SendCommandResponse>(
        `/api/admin/devices/${encodeURIComponent(deviceId)}/command`,
        { command, timeout_seconds: timeoutSeconds ?? 30 }
      ),
  });
}

// Start device authorization flow (same as CLI)
interface DeviceAuthStart {
  device_code: string;
  user_code: string;
  verification_uri: string;
  verification_uri_complete: string;
  expires_in: number;
  interval: number;
}

interface DeviceAuthPollResult {
  status: string;
  access_token?: string;
  refresh_token?: string;
  error?: string;
}

export async function startDeviceAuth(): Promise<DeviceAuthStart> {
  return api.post<DeviceAuthStart>("/api/auth/device");
}

export async function pollDeviceAuth(
  deviceCode: string
): Promise<DeviceAuthPollResult> {
  return api.post<DeviceAuthPollResult>("/api/auth/device/poll", {
    device_code: deviceCode,
  });
}

// OpenAPI spec for astro-server HTTP API
export function useAstroOpenAPISpec() {
  return useQuery({
    queryKey: adminKeys.astroOpenapi(),
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    queryFn: () => api.get<any>("/api/astro/openapi.json"),
    staleTime: Infinity,
  });
}

export interface JobKindInfo {
  kind: string;
  args_schema: Record<string, unknown>;
}

export function useJobKinds() {
  return useQuery({
    queryKey: adminKeys.jobKinds(),
    queryFn: () => api.get<{ kinds: JobKindInfo[] }>("/api/admin/jobs/kinds").then((r) => r.kinds),
    staleTime: Infinity,
  });
}

export function useTriggerJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ kind, args }: { kind: string; args: Record<string, unknown> }) =>
      api.post<{ job_id: number }>("/api/admin/jobs/trigger", { kind, args_json: args }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.jobStates() });
      qc.invalidateQueries({ queryKey: adminKeys.jobsAll() });
    },
  });
}

export interface JobStates {
  available: number;
  cancelled: number;
  completed: number;
  discarded: number;
  pending: number;
  retryable: number;
  running: number;
  scheduled: number;
}

export interface AdminQueue {
  count_available: number;
  count_running: number;
  name: string;
  paused_at: string | null;
  updated_at: string;
}

export interface AdminJob {
  id: number;
  args: Record<string, unknown>;
  attempt: number;
  attempted_at: string | null;
  created_at: string;
  errors: Array<{ at: string; attempt: number; error: string; trace: string }> | null;
  finalized_at: string | null;
  kind: string;
  max_attempts: number;
  priority: number;
  queue: string;
  scheduled_at: string;
  state: string;
}

export interface AdminJobsResponse {
  jobs: AdminJob[];
  next_before_id?: number;
  has_more: boolean;
}

export function useJobStates() {
  return useQuery({
    queryKey: adminKeys.jobStates(),
    queryFn: () => api.get<JobStates>("/api/admin/jobs/states"),
    refetchInterval: 10_000,
  });
}

export function useAdminJob(id: number | null) {
  return useQuery({
    queryKey: adminKeys.job(id ?? 0),
    queryFn: () => api.get<{ job: AdminJob }>(`/api/admin/jobs/${id}`).then((r) => r.job),
    enabled: id !== null,
    staleTime: 30_000,
  });
}

export function useAdminQueues() {
  return useQuery({
    queryKey: adminKeys.adminQueues(),
    queryFn: () => api.get<{ queues: AdminQueue[] }>("/api/admin/jobs/queues").then((r) => r.queues ?? []),
    refetchInterval: 10_000,
  });
}

export function useAdminJobs(params: { state?: string; kinds?: string[]; queue?: string; limit?: number; beforeId?: number; anchorId?: number }) {
  const paramKey = {
    state: params.state,
    queue: params.queue,
    kinds: params.kinds?.join(","),
    limit: String(params.limit ?? ""),
    beforeId: params.beforeId != null ? String(params.beforeId) : undefined,
    anchorId: params.anchorId != null ? String(params.anchorId) : undefined,
  };
  return useQuery({
    queryKey: adminKeys.jobs(paramKey),
    queryFn: () => {
      const qs = new URLSearchParams();
      if (params.state) qs.set("state", params.state);
      if (params.queue) qs.set("queue", params.queue);
      if (params.kinds?.length) params.kinds.forEach((k) => qs.append("kinds", k));
      if (params.limit) qs.set("limit", String(params.limit));
      if (params.beforeId != null) qs.set("before_id", String(params.beforeId));
      if (params.anchorId != null) qs.set("anchor_id", String(params.anchorId));
      return api.get<AdminJobsResponse>(`/api/admin/jobs?${qs}`).then((r) => ({
        jobs: r.jobs ?? [],
        next_before_id: r.next_before_id,
        has_more: r.has_more ?? false,
      }));
    },
    refetchInterval: params.state === "running" ? 5_000 : undefined,
  });
}

export function usePauseQueue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.post(`/api/admin/queues/${encodeURIComponent(name)}/pause`),
    onSuccess: () => qc.invalidateQueries({ queryKey: adminKeys.adminQueues() }),
  });
}

export function useResumeQueue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.post(`/api/admin/queues/${encodeURIComponent(name)}/resume`),
    onSuccess: () => qc.invalidateQueries({ queryKey: adminKeys.adminQueues() }),
  });
}

export function useCancelJobs() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: number[]) => api.post<{ cancelled: number }>("/api/admin/jobs/cancel", { ids }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.jobStates() });
      qc.invalidateQueries({ queryKey: adminKeys.jobsAll() });
    },
  });
}

export function useRetryJobs() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: number[]) => api.post<{ retried: number }>("/api/admin/jobs/retry", { ids }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.jobStates() });
      qc.invalidateQueries({ queryKey: adminKeys.jobsAll() });
    },
  });
}

interface QuotaRequest {
  id: string;
  account_id: string;
  account_name: string;
  feature_key: string;
  current_usage: number;
  current_quota: number;
  requested_amount: number;
  reason: string;
  status: string;
  requested_by: string;
  resolved_by: string;
  resolved_at: string;
  resolution_note: string;
  grant_amount: number;
  created_at: string;
}

export function useQuotaRequests(status?: string) {
  return useQuery({
    queryKey: adminKeys.quotaRequests(status),
    queryFn: () =>
      api.get<{ requests: QuotaRequest[]; count: number }>(
        `/api/admin/quota-requests${status ? `?status=${status}` : ""}`
      ),
  });
}

export function useApproveQuotaRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, grantAmount, note }: { id: string; grantAmount: number; note?: string }) =>
      api.post(`/api/admin/quota-requests/${encodeURIComponent(id)}/approve`, {
        grant_amount: grantAmount,
        note: note ?? "",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.quotaRequests() });
    },
  });
}

export function useDenyQuotaRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, note }: { id: string; note?: string }) =>
      api.post(`/api/admin/quota-requests/${encodeURIComponent(id)}/deny`, {
        note: note ?? "",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.quotaRequests() });
    },
  });
}

export type { QuotaRequest };

interface FeedbackSubmission {
  id: string;
  user_id: string;
  user_email: string;
  message: string;
  page_url: string;
  created_at: string;
}

export function useFeedback() {
  return useQuery({
    queryKey: adminKeys.feedback(),
    queryFn: () =>
      api.get<{ submissions: FeedbackSubmission[]; count: number }>(
        "/api/admin/feedback"
      ),
    refetchInterval: 30_000,
  });
}

export type { FeedbackSubmission };

export function useDeploymentEvents(id: string) {
  return useQuery({
    queryKey: adminKeys.deploymentEvents(id),
    queryFn: () =>
      api.get<GetDeploymentEventsResponse>(
        `/api/admin/deployments/${encodeURIComponent(id)}/events`
      ),
    enabled: !!id,
  });
}

export function useWakeUpDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(id)}/wakeup`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useStopDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(id)}/stop`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useDeploymentJobs(id: string) {
  return useQuery({
    queryKey: adminKeys.deploymentJobs(id),
    queryFn: () =>
      api.get<GetDeploymentJobsResponse>(
        `/api/admin/deployments/${encodeURIComponent(id)}/jobs`
      ),
    enabled: !!id,
    refetchInterval: 10_000,
  });
}

export function useReapplyDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<ReapplyDeploymentResponse>(
        `/api/admin/deployments/${encodeURIComponent(id)}/reapply`,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useRepairNormalizedSpec() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<{ status: string; workloads: number; services: number; ingresses: number }>(
        `/api/admin/deployments/${encodeURIComponent(id)}/repair-normalized`,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useRollbackDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, revision }: { id: string; revision: number }) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(id)}/rollback`, {
        revision,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function usePodEnv(id: string, pod: string) {
  return useQuery({
    queryKey: adminKeys.podEnv(id, pod),
    queryFn: () =>
      api.get<GetPodEnvResponse>(
        `/api/admin/pods/${encodeURIComponent(id)}/${encodeURIComponent(pod)}/env`
      ),
    enabled: !!id && !!pod,
  });
}

export function useClusters(enabledOnly?: boolean) {
  return useQuery({
    queryKey: adminKeys.clusters(enabledOnly),
    queryFn: () =>
      api.get<ListClustersResponse>(
        `/api/admin/clusters${enabledOnly ? "?enabled_only=true" : ""}`,
      ),
    refetchInterval: 10_000,
  });
}

export function useRegisterCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: RegisterClusterRequest) =>
      api.post<RegisterClusterResponse>("/api/admin/clusters", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...adminKeys.all, "clusters"] });
    },
  });
}

export function useEnableCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<EnableClusterResponse>(
        `/api/admin/clusters/${encodeURIComponent(id)}/enable`,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...adminKeys.all, "clusters"] });
    },
  });
}

export function useDisableCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<DisableClusterResponse>(
        `/api/admin/clusters/${encodeURIComponent(id)}/disable`,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...adminKeys.all, "clusters"] });
    },
  });
}

export function useDeregisterCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/api/admin/clusters/${encodeURIComponent(id)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...adminKeys.all, "clusters"] });
    },
  });
}

/** Fetches only when `enabled` — meant to fire after a failed deregister. */
export function useClusterBlockers(id: string, enabled: boolean) {
  return useQuery({
    queryKey: adminKeys.clusterBlockers(id),
    queryFn: () =>
      api.get<GetClusterBlockersResponse>(`/api/admin/clusters/${encodeURIComponent(id)}/blockers`),
    enabled,
  });
}

export function useUpdateCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: UpdateClusterRequest & { id: string }) =>
      api.put<UpdateClusterResponse>(
        `/api/admin/clusters/${encodeURIComponent(id)}`,
        body,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...adminKeys.all, "clusters"] });
    },
  });
}

export function useCheckClusterHealth() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<CheckClusterHealthResponse>(
        `/api/admin/clusters/${encodeURIComponent(id)}/health-check`,
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: [...adminKeys.all, "clusters"] });
    },
  });
}

export function useAlerts() {
  return useQuery({
    queryKey: adminKeys.alerts(),
    queryFn: () => api.get<ListAlertsResponse>("/api/admin/alerts"),
    refetchInterval: 5_000,
  });
}

export function useClearAlert() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      deployment_id: string;
      workload?: string;
      condition: string;
    }) => api.post("/api/admin/alerts/clear", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.alerts() });
    },
  });
}

export function useMuteAlert() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      deployment_id: string;
      condition: string;
      duration_seconds: number;
    }) => api.post("/api/admin/alerts/mute", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.alerts() });
    },
  });
}

export function useUnmuteAlert() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { deployment_id: string; condition: string }) =>
      api.post("/api/admin/alerts/unmute", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.alerts() });
    },
  });
}
