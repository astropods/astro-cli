import type { InsightsUserIdentity } from "@/lib/api";
import { isSlackUserId } from "./user-classification";

export interface SlackIdentityDisplay {
  primary: string;
  deepLink: string | undefined;
}

export function insightsUserIdentityKey(
  user: Pick<InsightsUserIdentity, "identity_key" | "user_id" | "slack_team_id">,
): string {
  if (user.identity_key) return user.identity_key;
  if (isSlackUserId(user.user_id) && user.slack_team_id) {
    return `slack:${user.slack_team_id}:${user.user_id}`;
  }
  return user.user_id;
}

export function countSlackRowsMissingDetails(
  users: Array<Pick<InsightsUserIdentity, "identity_key" | "user_id" | "slack_team_id" | "slack_display_name" | "slack_avatar_url">>,
): number {
  const missing = new Set<string>();
  for (const user of users) {
    if (!isSlackUserId(user.user_id)) continue;
    if (user.slack_team_id && (user.slack_display_name || user.slack_avatar_url)) continue;
    missing.add(insightsUserIdentityKey(user));
  }
  return missing.size;
}

export function slackIdentityDisplay(user: InsightsUserIdentity): SlackIdentityDisplay {
  const uid = user.user_id;
  const primary = user.slack_display_name || `Slack user - ${uid}`;
  const deepLink = user.slack_team_id ? `slack://user?team=${user.slack_team_id}&id=${uid}` : undefined;

  return { primary, deepLink };
}

export function identityRefFromUserID(userID: string): InsightsUserIdentity {
  return { user_id: userID };
}
