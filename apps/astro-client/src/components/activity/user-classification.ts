// Sentinels + recognizers used by Insights to bucket a trace's user_id
// into named-member / Slack-user / unidentified / unattributed rows.

export const ALL_USERS_KEY = "__all_users__";
export const UNATTRIBUTED_USER_KEY = "__unattributed__";
export const UNIDENTIFIED_USER_KEY = "__unidentified__";

export const ALL_USERS_COLOR = "var(--color-indigo-500)";
export const UNATTRIBUTED_COLOR = "var(--color-faint-foreground, var(--color-muted-foreground))";
export const UNIDENTIFIED_COLOR = "var(--warning)";

// Slack `user_id` is "U" followed by 8-11 uppercase alphanumeric chars
// (legacy IDs are 9 chars total = U + 8; modern workspaces issue up to 11
// total = U + 10; one extra char of headroom guards against future
// expansion). Tighter than this would risk dropping a real Slack user;
// looser would false-positive on any arbitrary `U…` string emitted by a
// custom SDK.
//
// The slack adapter writes this exact shape onto `msg.User.Id` for every
// unlinked Slack user (matching the format every historical Langfuse
// trace already carries). The workspace team_id is *not* embedded — the
// server attaches it as a separate response field via the
// slack_identity_mappings directory join, so a single Slack user has one
// aggregation key forever and the Insights "by people" view shows one
// row per human.
const SLACK_BARE_RE = /^U[A-Z0-9]{8,11}$/;

/**
 * True when uid looks like a Slack user_id — used to render the row with
 * the Slack icon and the deep-link affordance. Linked Slack users surface
 * as their WorkOS id and are not matched here.
 */
export function isSlackUserId(uid: string): boolean {
  return SLACK_BARE_RE.test(uid);
}
