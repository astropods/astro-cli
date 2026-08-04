import { useCallback, useEffect, useMemo } from "react";
import { useSearchParams } from "react-router";
import { useAuth } from "@/lib/auth";
import { canonicalizeUserResourceAccounts } from "@/lib/user-resource-scope";

export function useAccountFilterParam(): [
  string[],
  (accounts: string[]) => void,
] {
  const { accounts } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const requested = useMemo(() => searchParams.getAll("account"), [searchParams]);
  const key = JSON.stringify(requested);
  const memberships = useMemo(() => accounts.map((account) => account.name), [accounts]);

  const value = useMemo(
    () => canonicalizeUserResourceAccounts(requested, memberships),
    [memberships, requested],
  );

  const setValue = useCallback(
    (next: string[]) => {
      const normalized = canonicalizeUserResourceAccounts(next, memberships);
      setSearchParams(
        (current) => {
          const updated = new URLSearchParams(current);
          updated.delete("account");
          normalized.forEach((account) => updated.append("account", account));
          return updated;
        },
        { replace: true },
      );
    },
    [memberships, setSearchParams],
  );

  useEffect(() => {
    if (accounts.length === 0 || requested.length === 0 || key === JSON.stringify(value)) return;
    setValue(value);
  }, [accounts.length, key, requested.length, setValue, value]);

  return [value, setValue];
}
