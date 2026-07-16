import type {
  AgentDeployment,
  AgentDeploymentSummary,
  DeploymentRuntime,
  DeploymentStatus,
  DeploymentStatusValue,
} from "./api";

export const launchUnavailableMessage =
  "Launch is unavailable while we create your custom URL";

export const PAUSED_DEPLOYMENT_RECORD_STATUSES = ["stopped", "scaled_down", "Stopped"] as const;

export type PausedDeploymentRecordStatus =
  (typeof PAUSED_DEPLOYMENT_RECORD_STATUSES)[number];

export const PAUSED_DEPLOYMENT_STATUS_SEED = {
  value: "inactive",
  reason: "paused",
  details: "Deployment is paused",
} satisfies DeploymentStatus;

export function coercePausedDeploymentStatus(status: string): string {
  return PAUSED_DEPLOYMENT_RECORD_STATUSES.includes(status as PausedDeploymentRecordStatus)
    ? status
    : PAUSED_DEPLOYMENT_RECORD_STATUSES[0];
}

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
/** List filter: deployment spec includes web messaging (proxy-eligible). */
export function isChatListEligible(
  summary: AgentDeploymentSummary | null | undefined,
): boolean {
  return summary?.messaging_web_configured === true;
}

export function hasNewerBuild(deployment: {
  build_id?: string;
  latest_build_id?: string;
}): boolean {
  return !!deployment.latest_build_id && deployment.latest_build_id !== deployment.build_id;
}

export function withLatestBuildId(
  deployment: AgentDeployment | null | undefined,
  latestBuildId?: string,
) {
  return deployment && !deployment.latest_build_id && latestBuildId
    ? { ...deployment, latest_build_id: latestBuildId }
    : deployment;
}

/**
 * The chat composer's high-level state, derived from the deployment's coarse
 * status, its reason code, and the live messaging reachability. Drives both the
 * composer UI (enabled / paused banner / etc.) and whether dependent reads like
 * the inspector's agent/config fetch should run at all.
 *
 * - unknown: status hasn't loaded yet; we don't know if the agent is reachable.
 * - ready: active + messaging reachable; the user can chat.
 * - paused: intentionally stopped by the user.
 * - error: deployment is in an error state.
 * - starting: deploying / provisioning.
 * - stopped: inactive / undeploying (not paused).
 * - unreachable: cluster or messaging endpoint not reachable right now.
 *
 * Note on readiness layering: `starting` is derived from the deployment's
 * coarse status (primary agent-workload readiness), whereas chat actually also
 * needs the messaging sidecar. The messaging dependency is captured separately
 * via `runtime.messaging_reachable` (active → unreachable when the sidecar
 * isn't ready), so the two readiness signals don't silently collapse into one.
 */
export type ChatComposerState =
  | "unknown"
  | "ready"
  | "paused"
  | "error"
  | "starting"
  | "stopped"
  | "unreachable";

export function deriveChatComposerState(
  status: DeploymentStatus | null | undefined,
  runtime: DeploymentRuntime | null | undefined,
): ChatComposerState {
  // Status not loaded yet — this is genuinely unknown. The composer treats it
  // optimistically (renders enabled, no flicker), but dependent reads that must
  // not fire against a possibly-unreachable agent (e.g. the inspector's
  // agent/config) gate on a concrete state instead of this one.
  if (!status) return "unknown";
  if (status.reason === "cluster_unreachable") return "unreachable";

  switch (status.value) {
    case "active":
      // The Service can exist (status active) while the messaging sidecar isn't
      // actually ready; messaging_reachable is the live signal we have (Service
      // present AND messaging sidecar container ready — see DeploymentRuntime).
      return runtime?.messaging_reachable === false ? "unreachable" : "ready";
    case "deploying":
      return "starting";
    case "error":
      return "error";
    case "inactive":
      return status.reason === "paused" ? "paused" : "stopped";
    case "undeploying":
      return "stopped";
    default:
      return "ready";
  }
}

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
  return PAUSED_DEPLOYMENT_RECORD_STATUSES.some(
    (status) => status.toLowerCase() === s,
  );
}

/**
 * Returns the tooltip message to display when the Launch button is disabled,
 * based on the current deployment status.
 */
export function getLaunchDisabledMessage(
  statusValue?: DeploymentStatusValue | string,
): string {
  switch (statusValue) {
    case "deploying":
    case "pending":
      return "Agent is still deploying. Launch will be available once deployment is complete.";
    case "undeploying":
      return "Agent is being undeployed. Launch is temporarily unavailable.";
    case "error":
      return "Agent is in an error state. Please check the deployment status.";
    case "Stopped":
    case "inactive":
      return "Agent is paused. Resume the agent to launch.";
    default:
      return "Agent is not ready. Launch will be available once the agent is active.";
  }
}

/** Truncate a build id for display (8 chars); returns it unchanged when already short. */
export function shortBuildId(buildId: string): string {
  return buildId.length > 8 ? buildId.slice(0, 8) : buildId;
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
