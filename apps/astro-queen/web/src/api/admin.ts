import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { adminKeys } from "./keys";
import type {
  ListDeploymentsResponse,
  GetDeploymentResponse,
  ListAccountsResponse,
  ListBlueprintsResponse,
  GetBlueprintBuildsResponse,
  ClusterStatusResponse,
  GetPodLogsResponse,
  GetPodEnvResponse,
  ListConnectedDevicesResponse,
  SendCommandResponse,
  GetDeploymentEventsResponse,
  GetDeploymentJobsResponse,
  RefreshDriftReportResponse,
  ListClustersResponse,
  RegisterClusterRequest,
  RegisterClusterResponse,
  EnableClusterResponse,
  DisableClusterResponse,
  UpdateClusterRequest,
  UpdateClusterResponse,
  CheckClusterHealthResponse,
  InvalidateCachesResponse,
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

export function useDeployment(id: string, refetchInterval?: number) {
  return useQuery({
    queryKey: adminKeys.deployment(id),
    queryFn: () =>
      api.get<GetDeploymentResponse>(
        `/api/admin/deployments/${encodeURIComponent(id)}`
      ),
    enabled: !!id,
    refetchInterval,
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

export function useRenameAccount() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, newName }: { id: string; newName: string }) =>
      api.put(`/api/admin/accounts/${encodeURIComponent(id)}/rename`, {
        new_name: newName,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.accounts() });
    },
  });
}

export function useSetAccountCluster() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, clusterId }: { id: string; clusterId: string }) =>
      api.put(`/api/admin/accounts/${encodeURIComponent(id)}/cluster`, {
        cluster_id: clusterId,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.accounts() });
    },
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

export function useRiverUIStatus() {
  return useQuery({
    queryKey: adminKeys.riverUIStatus(),
    queryFn: () =>
      api.get<{ running: boolean }>("/api/admin/riverui/status"),
    refetchInterval: 5_000,
  });
}

export function useStartRiverUI() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<{ status: string }>("/api/admin/riverui/start"),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.riverUIStatus() });
    },
  });
}

export function useStopRiverUI() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<{ status: string }>("/api/admin/riverui/stop"),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.riverUIStatus() });
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

export function useBackfillResolvedKeys() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<{ backfilled_count: number }>("/api/admin/backfill-resolved-keys"),
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
      api.post(`/api/admin/deployments/${encodeURIComponent(id)}/reapply`),
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

export function useRefreshDriftReport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<RefreshDriftReportResponse>(
        `/api/admin/deployments/${encodeURIComponent(id)}/refresh-drift`,
      ),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: adminKeys.deployment(id) });
    },
  });
}

export function useSetAdapters() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, adapters }: { id: string; adapters: string[] }) =>
      api.post<{ status: string; adapters: string[] }>(
        `/api/admin/deployments/${encodeURIComponent(id)}/adapters`,
        { adapters },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
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
