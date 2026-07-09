import { useCallback, useEffect, useState } from "react";
import { useLastErrorLog } from "@/api/queries/deployments";

/** Aggregates per-container error messages reported by ContainerLogErrorProbe.
 *  Shared by the pod tile (indicator) and the pod detail panel (banner) so both
 *  read the same react-query-cached error lookups. */
export function useContainerErrors() {
  const [byContainer, setByContainer] = useState<Record<string, string | null>>({});
  const report = useCallback((container: string, message: string | null) => {
    setByContainer((prev) => (prev[container] === message ? prev : { ...prev, [container]: message }));
  }, []);
  return { byContainer, report };
}

/** First non-null error message across the given containers, or null. */
export function firstContainerError(
  byContainer: Record<string, string | null>,
  containers: string[],
): string | null {
  for (const c of containers) {
    if (byContainer[c]) return byContainer[c];
  }
  return null;
}

/** Invisible probe: reports one container's most recent error-level log message
 *  (or null when clean). Rendered once per container so the hook count stays
 *  stable, unlike calling the hook in a loop. */
export function ContainerLogErrorProbe({
  deploymentId,
  workloadName,
  container,
  onResult,
}: {
  deploymentId: string;
  workloadName: string;
  container: string;
  onResult: (container: string, message: string | null) => void;
}) {
  const { data } = useLastErrorLog(deploymentId, workloadName, container, true);
  const message = data && data.length > 0 ? data[0]?.message ?? "" : null;
  useEffect(() => {
    onResult(container, message);
  }, [container, message, onResult]);
  return null;
}
