import { useEffect, useMemo } from "react";
import { useSearchParams } from "react-router";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { orgScope } from "@/lib/user-resource-scope";

export function useOrgScope() {
  const { accounts } = useAuth();
  const { activeAccount, setActiveAccount } = useActiveAccount();
  const [searchParams, setSearchParams] = useSearchParams();
  const paramAccount = searchParams.get("account");

  useEffect(() => {
    if (!paramAccount || accounts.length === 0) return;
    if (accounts.some((account) => account.name === paramAccount) && paramAccount !== activeAccount) {
      setActiveAccount(paramAccount);
    }
    setSearchParams(
      (current) => {
        const next = new URLSearchParams(current);
        next.delete("account");
        next.delete("scope");
        return next;
      },
      { replace: true },
    );
  }, [accounts, activeAccount, paramAccount, setActiveAccount, setSearchParams]);

  const scope = useMemo(() => orgScope(activeAccount), [activeAccount]);

  return { account: activeAccount, setAccount: setActiveAccount, scope };
}
