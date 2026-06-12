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
// for a Slack-kind row. Username preferred over display name; falls back
// to the raw id. Deep link is omitted when the directory didn't surface a
// team_id (the slack:// URL is invalid without one).
export function slackIdentityDisplay(
  user: Pick<UserIdentity, "user_id" | "user_details">,
): SlackIdentityDisplay {
  const uid = user.user_id;
  const details = user.user_details;
  const primary = details?.username || details?.display_name || `Slack user - ${uid}`;
  const deepLink = details?.team_id ? `slack://user?team=${details.team_id}&id=${uid}` : undefined;
  return { primary, deepLink };
}

// identityRefFromUserID builds a placeholder UserIdentity from a raw
// user_id alone. Used by trace-detail and deployment-summary callers
// that only have the user_id string and need to feed a UserIdentity-
// shaped value to UI components.
export function identityRefFromUserID(userID: string): UserIdentity {
  return { user_id: userID, user_details: { kind: classifyUserID(userID) } };
}
