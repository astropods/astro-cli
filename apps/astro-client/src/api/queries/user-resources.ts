import type { UserResourceResponse } from '@/lib/api';
import type { UserResourceScopeSelection } from '@/lib/user-resource-scope';

export const USER_RESOURCE_PAGE_SIZE = 50;
export const USER_RESOURCE_STALE_TIME_MS = 30_000;

export function nextUserResourceCursor(lastPage: UserResourceResponse) {
  return lastPage.page.next_cursor || undefined;
}

export function isUserResourceQueryEnabled(
  scope: UserResourceScopeSelection,
  enabled: boolean,
) {
  return enabled && scope.accounts.length > 0;
}
