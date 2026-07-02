import type { UserDetailsKind, UserIdentity } from "@/lib/api";

export interface SlackIdentityDisplay {
  primary: string;
  deepLink: string | undefined;
}

const SLACK_BARE_RE = /^U[A-Z0-9]{8,11}$/;

function isSlackUserId(uid: string): boolean {
  return SLACK_BARE_RE.test(uid);
}

// classifyUserID mirrors the server-side discriminator (handlers/observability_langfuse.go).
// Used when the client builds a placeholder UserIdentity from a raw user_id
// (e.g. older deployments_summary payloads that only carry users_used: string[]).
export function classifyUserID(userID: string): UserDetailsKind {
  if (!userID) return "unknown";
  if (isSlackUserId(userID)) return "slack";
  if (userID.startsWith("user_")) return "astro";
  return "unknown";
}

// insightsUserIdentityKey returns the stable React key for a user row.
// Slack users with a known workspace use `slack:<team>:<uid>` so the same
// Slack id observed in two workspaces stays distinct; everything else
// keys on user_id directly.
export function insightsUserIdentityKey(
  user: Pick<UserIdentity, "user_id" | "user_details">,
): string {
  const details = user.user_details;
  if (details?.kind === "slack" && details.team_id) {
    return `slack:${details.team_id}:${user.user_id}`;
  }
  return user.user_id;
}

// slackIdentityDisplay picks the human-facing label + slack:// deep link
// for a Slack-kind row. Keep the display-name fallback logic aligned with
// apps/astro-server/handlers/slack_display.go for trace rows that do not
// receive the server-shaped Insights label.
// Deep link is omitted when the directory didn't surface a team_id.
export function slackIdentityDisplay(
  user: Pick<UserIdentity, "user_id" | "user_details">,
): SlackIdentityDisplay {
  const uid = user.user_id;
  const details = user.user_details;
  const primary = slackDisplayNameFromProfile(details?.display_name, details?.username) || `Slack user - ${uid}`;
  const deepLink = details?.team_id ? `slack://user?team=${details.team_id}&id=${uid}` : undefined;
  return { primary, deepLink };
}

function slackDisplayNameFromProfile(displayName?: string, username?: string): string {
  const trimmedDisplayName = displayName?.trim() ?? "";
  const trimmedUsername = username?.trim() ?? "";
  if (!trimmedDisplayName) {
    return slackDisplayNameFromUsername(trimmedUsername);
  }
  if (isStaleSlackHandleDisplayName(trimmedDisplayName, trimmedUsername)) {
    return slackDisplayNameFromUsername(trimmedUsername);
  }
  return trimmedDisplayName;
}

function isStaleSlackHandleDisplayName(displayName: string, username: string): boolean {
  if (!displayName || !username || displayName !== username || displayName !== displayName.toLowerCase()) {
    return false;
  }
  const parts = displayName.split(/[._-]/).filter(Boolean);
  if (parts.length < 2) {
    return false;
  }
  return parts.every((part) => Array.from(part).length > 1);
}

function slackDisplayNameFromUsername(username: string): string {
  if (!username) return "";
  const parts = username.split(/[._-]/).map((part) => part.trim()).filter(Boolean);
  if (parts.length === 0) return username;
  return parts.map(titleSlackNamePart).join(" ");
}

function titleSlackNamePart(part: string): string {
  const lower = part.toLowerCase();
  return lower ? lower[0].toUpperCase() + lower.slice(1) : "";
}

// identityRefFromUserID builds a placeholder UserIdentity from a raw
// user_id alone. Used by trace-detail and deployment-summary callers
// that only have the user_id string and need to feed a UserIdentity-
// shaped value to UI components.
export function identityRefFromUserID(userID: string): UserIdentity {
  return { user_id: userID, user_details: { kind: classifyUserID(userID) } };
}
