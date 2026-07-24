import { useCallback, useEffect, useMemo } from "react";
import { useSearchParams } from "react-router";
import { useAuth } from "@/lib/auth";

export function useAccountFilterParam(): [
  string[],
  (accounts: string[]) => void,
] {
  const { accounts } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const requested = useMemo(() => searchParams.getAll("account"), [searchParams]);
  const key = JSON.stringify(requested);

  const value = useMemo(() => {
    const known = new Set(accounts.map((account) => account.name));
    return requested.filter((name) => known.has(name));
  }, [accounts, requested]);

  const setValue = useCallback(
    (next: string[]) => {
      setSearchParams(
        (current) => {
          const updated = new URLSearchParams(current);
          updated.delete("account");
          next.forEach((account) => updated.append("account", account));
          return updated;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  useEffect(() => {
    if (accounts.length === 0 || requested.length === 0 || key === JSON.stringify(value)) return;
    setValue(value);
  }, [accounts.length, key, requested.length, setValue, value]);

  return [value, setValue];
}
