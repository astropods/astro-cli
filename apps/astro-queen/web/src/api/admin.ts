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
} from "@/types/admin";

export function useDeployments() {
  return useQuery({
    queryKey: adminKeys.deployments(),
    queryFn: () => api.get<ListDeploymentsResponse>("/api/admin/deployments"),
  });
}

export function useDeployment(namespace: string) {
  return useQuery({
    queryKey: adminKeys.deployment(namespace),
    queryFn: () =>
      api.get<GetDeploymentResponse>(
        `/api/admin/deployments/${encodeURIComponent(namespace)}`
      ),
    enabled: !!namespace,
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

export function usePodLogs(namespace: string, pod: string) {
  return useQuery({
    queryKey: adminKeys.podLogs(namespace, pod),
    queryFn: () =>
      api.get<GetPodLogsResponse>(
        `/api/admin/pods/${encodeURIComponent(namespace)}/${encodeURIComponent(pod)}/logs`
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

// Exchange refresh token for access token via admin gRPC
export function useGetAuthToken() {
  return useMutation({
    mutationFn: (refreshToken?: string) =>
      api.post<{ access_token: string; refresh_token: string }>(
        "/api/auth/token",
        refreshToken ? { refresh_token: refreshToken } : {}
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
