export interface UserResourceScopeSelection {
  accounts: string[];
  all: boolean;
}

export function canonicalizeUserResourceAccounts(
  requested: string[],
  memberships: string[],
): string[] {
  const accounts = [...new Set(memberships)].sort();
  const known = new Set(accounts);
  const selected = [...new Set(requested)]
    .filter((account) => known.has(account))
    .sort();
  return selected.length === accounts.length ? [] : selected;
}

/**
 * Resolves the page-local account filter against the memberships already
 * returned by /me. An empty filter is the explicit all-memberships scope.
 */
export function resolveUserResourceScope(
  requested: string[],
  memberships: string[],
): UserResourceScopeSelection {
  const accounts = [...new Set(memberships)].sort();
  const selected = canonicalizeUserResourceAccounts(requested, accounts);
  return selected.length > 0
    ? { accounts: selected, all: false }
    : { accounts, all: true };
}

export function resolvePageAccount(
  requested: string | null,
  memberships: string[],
  fallback: string,
): string {
  return requested && memberships.includes(requested) ? requested : fallback;
}
