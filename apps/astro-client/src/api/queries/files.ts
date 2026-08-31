/** TanStack Query bindings for the agent files API (GET/POST/PUT/DELETE
 *  /deployments/:id/files). Backed by the deployment's persistent disk today;
 *  the same hooks work unchanged once a presigned object store lands. */
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { downloadBlob } from "@/lib/download";
import { fileKeys } from "./keys";

export function useDeploymentFiles(deploymentId: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: fileKeys.all(deploymentId),
    queryFn: () => api.listDeploymentFiles(deploymentId),
    enabled: enabled && !!deploymentId,
  });
}

/** Volume capacity for the deployment's file store. Event-driven, not polled:
 *  refreshed by invalidation on file upload/delete and chat-turn finish. */
export function useDeploymentStorageUsage(deploymentId: string, enabled = true) {
  const api = useApiClient();
  return useQuery({
    queryKey: fileKeys.usage(deploymentId),
    queryFn: () => api.getDeploymentStorageUsage(deploymentId),
    enabled: enabled && !!deploymentId,
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
      void queryClient.invalidateQueries({ queryKey: fileKeys.all(deploymentId) });
      void queryClient.invalidateQueries({ queryKey: fileKeys.usage(deploymentId) });
    },
  });
}

export function useDeleteDeploymentFile(deploymentId: string) {
  const api = useApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (key: string) => api.deleteDeploymentFile(deploymentId, key),
    onSuccess: (_result, key) => {
      void queryClient.invalidateQueries({ queryKey: fileKeys.all(deploymentId) });
      void queryClient.invalidateQueries({ queryKey: fileKeys.usage(deploymentId) });
      void queryClient.resetQueries({
        queryKey: fileKeys.content(deploymentId, key),
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
      downloadBlob(await api.downloadDeploymentFile(deploymentId, key), name);
    },
  });
}

export const MAX_PREVIEW_BYTES = 8 * 1024 * 1024;

export function useDeploymentFilePreview(deploymentId: string, key: string) {
  const api = useApiClient();
  // gcTime: 0 buys a memory bound proportional to what is on screen, at the
  // cost of a re-download when a thumbnail scrolls back in. A time-based window
  // has no count cap, so one fast scroll would hold every blob it passed.
  const { data: blob } = useQuery({
    queryKey: fileKeys.content(deploymentId, key),
    queryFn: ({ signal }) => api.downloadDeploymentFile(deploymentId, key, signal),
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
  });

  const [url, setUrl] = useState<string>();
  useEffect(() => {
    if (!blob) {
      setUrl(undefined);
      return;
    }
    const objectUrl = URL.createObjectURL(blob);
    setUrl(objectUrl);
    return () => URL.revokeObjectURL(objectUrl);
  }, [blob]);

  return url;
}
