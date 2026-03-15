import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { adminKeys } from "./keys";
import type {
  ListDeploymentsResponse,
  GetDeploymentResponse,
  ListAccountsResponse,
  ListAgentsResponse,
  GetAgentBuildsResponse,
  ClusterStatusResponse,
  GetPodLogsResponse,
  GetPodEnvResponse,
  ListConnectedDevicesResponse,
  SendCommandResponse,
  GetDeploymentEventsResponse,
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

export function useDeployment(namespace: string, refetchInterval?: number) {
  return useQuery({
    queryKey: adminKeys.deployment(namespace),
    queryFn: () =>
      api.get<GetDeploymentResponse>(
        `/api/admin/deployments/${encodeURIComponent(namespace)}`
      ),
    enabled: !!namespace,
    refetchInterval,
  });
}

export function useDeleteDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (namespace: string) =>
      api.del(`/api/admin/deployments/${encodeURIComponent(namespace)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useRestartDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (namespace: string) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(namespace)}/restart`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
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

export function useAgents() {
  return useQuery({
    queryKey: adminKeys.agents(),
    queryFn: () => api.get<ListAgentsResponse>("/api/admin/agents"),
  });
}

export function useAgentBuilds(account: string, name: string) {
  return useQuery({
    queryKey: adminKeys.agentBuilds(account, name),
    queryFn: () =>
      api.get<GetAgentBuildsResponse>(
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

export function usePodLogs(namespace: string, pod: string, container?: string) {
  return useQuery({
    queryKey: [...adminKeys.podLogs(namespace, pod), container ?? ""],
    queryFn: () =>
      api.get<GetPodLogsResponse>(
        `/api/admin/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/logs${container ? `?container=${encodeURIComponent(container)}` : ""}`
      ),
    enabled: !!namespace && !!pod,
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

export function useDeploymentEvents(namespace: string) {
  return useQuery({
    queryKey: adminKeys.deploymentEvents(namespace),
    queryFn: () =>
      api.get<GetDeploymentEventsResponse>(
        `/api/admin/deployments/${encodeURIComponent(namespace)}/events`
      ),
    enabled: !!namespace,
  });
}

export function useWakeUpDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (namespace: string) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(namespace)}/wakeup`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useReapplyDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (namespace: string) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(namespace)}/reapply`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function useRollbackDeployment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ namespace, revision }: { namespace: string; revision: number }) =>
      api.post(`/api/admin/deployments/${encodeURIComponent(namespace)}/rollback`, {
        revision,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: adminKeys.deployments() });
    },
  });
}

export function usePodEnv(namespace: string, pod: string) {
  return useQuery({
    queryKey: adminKeys.podEnv(namespace, pod),
    queryFn: () =>
      api.get<GetPodEnvResponse>(
        `/api/admin/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/env`
      ),
    enabled: !!namespace && !!pod,
  });
}
