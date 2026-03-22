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
export function getPodDisplayName(stableName: string, agentName?: string): string {
  if (agentName && stableName.startsWith(agentName + "-")) {
    const suffix = stableName.slice(agentName.length + 1);
    return `${agentName} ${suffix.replace(/-/g, " ")}`;
  }
  return stableName;
}
