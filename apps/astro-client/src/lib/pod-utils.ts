/**
 * Derives a stable base name from a Kubernetes pod name by stripping
 * the trailing ReplicaSet hash and pod hash segments.
 *
 * e.g. "myagent-agent-7f8b9c4d5-x2k9p" → "myagent-agent"
 */
export function getPodStableName(podName: string): string {
  const parts = podName.split("-");
  if (parts.length <= 2) return podName;
  return parts.slice(0, -2).join("-");
}

/**
 * Converts a pod stable name into a display name. Dashes within the
 * agent template name are preserved; dashes after it become spaces.
 *
 * e.g. ("clawbot-ai-agent", "clawbot-ai") → "clawbot-ai agent"
 *      ("clawbot-ai-otel-collector", "clawbot-ai") → "clawbot-ai otel collector"
 */
export function getPodDisplayName(stableName: string, agentName?: string): string {
  if (agentName && stableName.startsWith(agentName + "-")) {
    const suffix = stableName.slice(agentName.length + 1);
    return `${agentName} ${suffix.replace(/-/g, " ")}`;
  }
  return stableName;
}
