import type { AgentDeployment, AgentDeploymentSummary } from "./api";

export const launchUnavailableMessage =
  "Launch is unavailable while we create your custom URL";

export function getMessagingEndpoint(deployment: AgentDeployment | AgentDeploymentSummary | null | undefined) {
  return deployment?.external_urls?.find((u) => u.type === "messaging");
}

/**
 * Whether the messaging Launch button should be clickable.
 * - The URL must exist (record).
 * - The messaging sidecar must be part of the spec (record).
 * - The endpoint itself must report ready (server omits ready when false).
 *
 * Note: the server-side liveness probe (messaging_reachable) lives on the
 * runtime endpoint; callers that need to gate on it should additionally
 * check `runtime.messaging_reachable !== false` after fetching the runtime
 * view. Most call sites only need the record-level gating below.
 */
export function isLaunchReady(deployment: AgentDeployment | null | undefined): boolean {
  const messaging = getMessagingEndpoint(deployment);
  if (!messaging?.url) return false;
  if (deployment?.messaging_configured === false) return false;
  return messaging.ready === true;
}

// Pause/stop state is a pure DB-status concern; check the raw enum from the
// record. Used by AgentStatusToggle for the toggle's checked/unchecked state.
export function isPausedState(deployment: AgentDeployment): boolean {
  const s = deployment.status?.toLowerCase() ?? "";
  return s === "stopped" || s === "scaled_down";
}

export function formatRelativeTime(dateStr: string): string {
  const diffSecs = Math.round((new Date(dateStr).getTime() - Date.now()) / 1000);
  const diffMins = Math.round(diffSecs / 60);
  const diffHours = Math.round(diffMins / 60);
  const diffDays = Math.round(diffHours / 24);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (Math.abs(diffSecs) < 60) return "just now";
  if (Math.abs(diffMins) < 60) return rtf.format(diffMins, "minute");
  if (Math.abs(diffHours) < 24) return rtf.format(diffHours, "hour");
  return rtf.format(diffDays, "day");
}

export function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}


export function formatDaysActive(isoString: string): string {
  const days = Math.floor((Date.now() - new Date(isoString).getTime()) / (1000 * 60 * 60 * 24));
  if (days === 0) return "< 1 day";
  if (days === 1) return "1 day";
  return `${days} days`;
}
