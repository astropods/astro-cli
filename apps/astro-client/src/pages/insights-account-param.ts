import { useCallback, useEffect, useMemo } from "react";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { resolvePageAccount } from "@/lib/user-resource-scope";

export function resolveInsightsScopeAccount(
  paramAccount: string | null,
  accountNames: string[],
  activeAccount: string,
): string {
  return resolvePageAccount(paramAccount, accountNames, activeAccount);
}

export function removeStaleInsightsAccountParam(
  params: URLSearchParams,
  accountNames: string[],
): URLSearchParams | null {
  const account = params.get("account");
  if (!account || accountNames.length === 0 || accountNames.includes(account)) return null;
  const next = new URLSearchParams(params);
  next.delete("account");
  return next;
}

/** Resolves which account an Insights surface is scoped to, and the setter the
 *  scope switcher calls. Shared because the rules are subtle — the param is
 *  dropped when it equals the active account, and a param naming an account the
 *  caller is not a member of is cleaned off the URL — and every page reading it
 *  has to agree or a link between them silently changes account. */
export function useInsightsScopeAccount(
  searchParams: URLSearchParams,
  setSearchParams: (
    next: URLSearchParams | ((previous: URLSearchParams) => URLSearchParams),
    opts?: { replace?: boolean },
  ) => void,
) {
  const { accounts } = useAuth();
  const { activeAccount } = useActiveAccount();

  const accountNames = useMemo(() => accounts.map((a) => a.name), [accounts]);
  const paramAccount = searchParams.get("account");
  const account = resolveInsightsScopeAccount(paramAccount, accountNames, activeAccount);

  useEffect(() => {
    const next = removeStaleInsightsAccountParam(searchParams, accountNames);
    if (next) setSearchParams(next, { replace: true });
  }, [accountNames, paramAccount, searchParams, setSearchParams]);

  const setScopeAccount = useCallback((next: string) => {
    setSearchParams((previous) => {
      if (next === activeAccount) previous.delete("account");
      else previous.set("account", next);
      return previous;
    }, { replace: true });
  }, [activeAccount, setSearchParams]);

  return { account, paramAccount, setScopeAccount };
}
