/**
 * @deprecated Use WorkloadDetail.name directly — the server now provides workload names.
 * Kept for backward compatibility.
 */
export function getPodStableName(podName: string): string {
  const parts = podName.split("-");
  if (parts.length <= 2) return podName;
  return parts.slice(0, -2).join("-");
}

/**
 * @deprecated Use WorkloadDetail.component directly — the server now provides component names.
 * Kept for backward compatibility.
 */
export function getPodDisplayName(stableName: string, blueprintName?: string): string {
  if (blueprintName && stableName.startsWith(blueprintName + "-")) {
    const suffix = stableName.slice(blueprintName.length + 1);
    return `${blueprintName} ${suffix.replace(/-/g, " ")}`;
  }
  return stableName;
}
