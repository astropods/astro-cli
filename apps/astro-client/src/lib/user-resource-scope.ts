export interface UserResourceScopeSelection {
  accounts: string[];
  all: boolean;
}

export function orgScope(account: string): UserResourceScopeSelection {
  return { accounts: account ? [account] : [], all: false };
}

export function resolvePageAccount(
  requested: string | null,
  memberships: string[],
  fallback: string,
): string {
  return requested && memberships.includes(requested) ? requested : fallback;
}
