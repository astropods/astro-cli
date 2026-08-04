import { useCallback, useEffect, useMemo } from "react";
import { useSearchParams } from "react-router";
import { useAuth } from "@/lib/auth";
import { canonicalizeUserResourceAccounts } from "@/lib/user-resource-scope";
import { usePersistentSearchParams } from "./use-persistent-search-params";

const ACCOUNT_FILTER_PARAMS = ["account", "scope"] as const;

type AccountFilterParamState = [
  value: string[],
  setValue: (accounts: string[]) => void,
  hasExplicitSelection: boolean,
  resetValue: () => void,
];

function useAccountFilterParamState(
  paramsState: ReturnType<typeof useSearchParams>,
): AccountFilterParamState {
  const { accounts } = useAuth();
  const [searchParams, setSearchParams] = paramsState;
  const requested = useMemo(() => searchParams.getAll("account"), [searchParams]);
  const key = JSON.stringify(requested);
  const memberships = useMemo(() => accounts.map((account) => account.name), [accounts]);
  const knownRequested = useMemo(
    () => [...new Set(requested)].filter((account) => memberships.includes(account)),
    [memberships, requested],
  );
  const personalAccount =
    accounts.find((account) => account.type === "personal")?.name ?? memberships[0];
  const explicitAll = requested.length === 0 && searchParams.get("scope") === "all";
  const hasExplicitSelection = requested.length > 0 || explicitAll;

  const value = useMemo(
    () => {
      if (explicitAll) return [];
      if (requested.length > 0) {
        if (knownRequested.length === 0) {
          return personalAccount ? [personalAccount] : [];
        }
        return canonicalizeUserResourceAccounts(knownRequested, memberships);
      }
      return personalAccount ? [personalAccount] : [];
    },
    [explicitAll, knownRequested, memberships, personalAccount, requested.length],
  );

  const setValue = useCallback(
    (next: string[]) => {
      const normalized = canonicalizeUserResourceAccounts(next, memberships);
      setSearchParams(
        (current) => {
          const updated = new URLSearchParams(current);
          updated.delete("account");
          updated.delete("scope");
          if (normalized.length > 0) {
            normalized.forEach((account) => updated.append("account", account));
          } else {
            updated.set("scope", "all");
          }
          return updated;
        },
        { replace: true },
      );
    },
    [memberships, setSearchParams],
  );

  const resetValue = useCallback(() => {
    setSearchParams(
      (current) => {
        const updated = new URLSearchParams(current);
        updated.delete("account");
        updated.delete("scope");
        return updated;
      },
      { replace: true },
    );
  }, [setSearchParams]);

  useEffect(() => {
    if (accounts.length === 0 || requested.length === 0 || key === JSON.stringify(value)) return;
    const sanitized = knownRequested.length > 0
      ? value
      : personalAccount
        ? [personalAccount]
        : [];
    setValue(sanitized);
  }, [accounts.length, key, knownRequested.length, personalAccount, requested.length, setValue, value]);

  return [value, setValue, hasExplicitSelection, resetValue];
}

export function useAccountFilterParam(): AccountFilterParamState {
  return useAccountFilterParamState(useSearchParams());
}

export function usePersistentAccountFilterParam(storageScope: string): AccountFilterParamState {
  return useAccountFilterParamState(
    usePersistentSearchParams(storageScope, ACCOUNT_FILTER_PARAMS, { atomic: true }),
  );
}
