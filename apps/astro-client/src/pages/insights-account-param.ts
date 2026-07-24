export function resolveInsightsScopeAccount(
  paramAccount: string | null,
  accountNames: string[],
  activeAccount: string,
): string {
  return paramAccount && accountNames.includes(paramAccount) ? paramAccount : activeAccount;
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
