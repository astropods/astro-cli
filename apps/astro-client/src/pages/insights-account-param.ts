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
