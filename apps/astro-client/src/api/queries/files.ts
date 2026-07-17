/** TanStack Query bindings for the agent files API (GET/POST/PUT/DELETE
 *  /deployments/:id/files). Backed by the deployment's persistent disk today;
 *  the same hooks work unchanged once a presigned object store lands. */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiRequestError } from "@/lib/api";
import { useApiClient } from "@/lib/api-context";
import { fileKeys } from "./keys";

export function useDeploymentFiles(deploymentId: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: fileKeys.all(deploymentId),
    queryFn: () => api.listDeploymentFiles(deploymentId),
    enabled: enabled && !!deploymentId,
  });
}

/** Volume capacity for the deployment's file store, polled on a slow cadence
 *  (usage changes gradually) so the storage-capacity banner stays current
 *  without hammering the sidecar. Failures are swallowed by callers — a missing
 *  reading just hides the banner. */
export function useDeploymentStorageUsage(deploymentId: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: fileKeys.usage(deploymentId),
    queryFn: () => api.getDeploymentStorageUsage(deploymentId),
    enabled: enabled && !!deploymentId,
    // Stop polling once files are unavailable (404 = no web adapter).
    refetchInterval: (query) =>
      query.state.error instanceof ApiRequestError &&
      query.state.error.status === 404
        ? false
        : 60_000,
    staleTime: 30_000,
    retry: false,
  });
}

export function useUploadDeploymentFile(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => api.uploadDeploymentFile(deploymentId, file),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: fileKeys.all(deploymentId),
      });
    },
  });
}

export function useDeleteDeploymentFile(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (key: string) => api.deleteDeploymentFile(deploymentId, key),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: fileKeys.all(deploymentId),
      });
    },
  });
}

/** Fetches the file bytes and triggers a browser download. Not a query — it's a
 *  one-shot imperative action driven by a user click. */
export function useDownloadDeploymentFile(deploymentId: string) {
  const api = useApiClient();
  return useMutation({
    mutationFn: async ({ key, name }: { key: string; name: string }) => {
      const blob = await api.downloadDeploymentFile(deploymentId, key);
      const objectUrl = URL.createObjectURL(blob);
      try {
        const anchor = document.createElement("a");
        anchor.href = objectUrl;
        anchor.download = name;
        document.body.appendChild(anchor);
        anchor.click();
        anchor.remove();
      } finally {
        URL.revokeObjectURL(objectUrl);
      }
    },
  });
}
