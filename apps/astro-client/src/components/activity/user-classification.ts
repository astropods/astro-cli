// Domain logic + sentinels for classifying a trace's user_id into one of three
// buckets that the Insights surfaces share (filter bar, chart, top spenders).

export const ALL_USERS_KEY = "__all_users__";
export const UNATTRIBUTED_USER_KEY = "__unattributed__";
export const UNAUTHORIZED_USER_KEY = "__unauthorized__";

export const ALL_USERS_COLOR = "var(--color-indigo-500)";
export const UNATTRIBUTED_COLOR = "var(--color-faint-foreground, var(--color-muted-foreground))";
export const UNAUTHORIZED_COLOR = "var(--warning)";

export function classifyUserId(uid: string | null | undefined, memberIds: Set<string>): string {
  if (!uid) return UNATTRIBUTED_USER_KEY;
  return memberIds.has(uid) ? uid : UNAUTHORIZED_USER_KEY;
}
