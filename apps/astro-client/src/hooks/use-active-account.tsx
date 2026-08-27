import { createContext, useContext, useState, useCallback, useEffect, useRef, useTransition, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useRevalidator, useRouteLoaderData } from "react-router";
import { toast } from "sonner";
import { useAuth } from "@/lib/auth";
import { setOrgSwitchTarget } from "@/lib/org-switch-progress";
import { ACTIVE_ACCOUNT_COOKIE } from "@/lib/active-account";

interface ActiveAccountContextValue {
  activeAccount: string;
  setActiveAccount: (account: string) => void;
}

const ActiveAccountContext = createContext<ActiveAccountContextValue | null>(null);

// One-year persistence, matching root.tsx's theme cookie. `Secure` is added
// when served over HTTPS so the cookie never leaves the browser on an
// unencrypted connection — local dev (`http://localhost`) still works
// because the flag is conditional on `location.protocol`.
function secureFlag(): string {
  return typeof location !== "undefined" && location.protocol === "https:" ? ";Secure" : "";
}

function writeActiveAccountCookie(account: string) {
  document.cookie = `${ACTIVE_ACCOUNT_COOKIE}=${encodeURIComponent(account)};path=/;max-age=31536000;SameSite=Lax${secureFlag()}`;
}

function clearActiveAccountCookie() {
  document.cookie = `${ACTIVE_ACCOUNT_COOKIE}=;path=/;max-age=0;SameSite=Lax${secureFlag()}`;
}

export function ActiveAccountProvider({ children }: { children: ReactNode }) {
  const { accounts, personalAccount, organizationId, switchOrg } = useAuth();
  const revalidator = useRevalidator();
  const queryClient = useQueryClient();
  // Source initial value from the root loader (cookie-derived) rather than
  // localStorage so SSR and first client render agree — prevents a hydration
  // flash on the switcher.
  const rootData = useRouteLoaderData("root") as { activeAccount?: string } | undefined;
  const ssrAccount = rootData?.activeAccount ?? "";

  // Client override makes setActiveAccount feel instant; the revalidator
  // refreshes loader data underneath.
  const [override, setOverride] = useState<string | null>(null);
  const [isAccountPending, startAccountTransition] = useTransition();
  const validOverride = override && accounts.some((a) => a.name === override) ? override : null;
  const activeAccount = validOverride || ssrAccount || personalAccount?.name || "";
  const accountSwitchTargetRef = useRef<string | null>(null);

  const persistActiveAccount = useCallback((accountName: string) => {
    if (accountName === personalAccount?.name) {
      clearActiveAccountCookie();
      return;
    }
    writeActiveAccountCookie(accountName);
  }, [personalAccount?.name]);

  const setActiveAccount = useCallback((accountName: string) => {
    const target = accounts.find((a) => a.name === accountName);
    const switching = !!target && accountName !== activeAccount;

    if (!switching) {
      persistActiveAccount(accountName);
      setOverride(accountName);
      revalidator.revalidate();
      return;
    }

    accountSwitchTargetRef.current = accountName;
    setOrgSwitchTarget(accountName);

    const rescope = target.organization_id && target.organization_id !== organizationId
      ? switchOrg(target.organization_id)
      : Promise.resolve();

    void rescope.then(
      () => {
        if (accountSwitchTargetRef.current !== accountName) return;
        persistActiveAccount(accountName);
        revalidator.revalidate();
        startAccountTransition(() => setOverride(accountName));
      },
      () => {
        if (accountSwitchTargetRef.current !== accountName) return;
        accountSwitchTargetRef.current = null;
        setOrgSwitchTarget(null);
        toast.error(`Couldn't switch to ${target.display_name || target.name}. Try again.`);
      },
    );
  }, [accounts, activeAccount, organizationId, persistActiveAccount, revalidator, switchOrg]);

  // Keep org-switch progress active until revalidation, the account override
  // transition, and any account-scoped fetches have settled.
  useEffect(() => {
    const target = accountSwitchTargetRef.current;
    if (!target || activeAccount !== target) return;

    let sawFetch = false;
    let pendingClear = false;

    function accountFetching() {
      return queryClient.isFetching({
        predicate: (q) => q.queryKey.includes(target),
      });
    }

    function finishSwitch() {
      if (accountSwitchTargetRef.current !== target) return;
      accountSwitchTargetRef.current = null;
      setOrgSwitchTarget(null);
    }

    function scheduleWarmCacheClear() {
      if (pendingClear) return;
      pendingClear = true;
      // Let the progress bar paint and the outlet commit before clearing on warm cache.
      requestAnimationFrame(() => {
        requestAnimationFrame(finishSwitch);
      });
    }

    function checkDone() {
      if (revalidator.state !== "idle") return;
      if (isAccountPending) return;
      if (accountFetching() > 0) sawFetch = true;
      if (sawFetch) {
        if (accountFetching() === 0) finishSwitch();
        return;
      }
      scheduleWarmCacheClear();
    }

    checkDone();
    const unsub = queryClient.getQueryCache().subscribe(checkDone);
    return () => unsub();
  }, [activeAccount, isAccountPending, queryClient, revalidator.state]);

  // A switch cannot finish once this tree is gone, and the target lives in a
  // module-level store, so drop it rather than leave the app looking busy.
  useEffect(() => () => {
    accountSwitchTargetRef.current = null;
    setOrgSwitchTarget(null);
  }, []);

  return (
    <ActiveAccountContext.Provider value={{ activeAccount, setActiveAccount }}>
      {children}
    </ActiveAccountContext.Provider>
  );
}

export function useActiveAccount() {
  const ctx = useContext(ActiveAccountContext);
  if (!ctx) throw new Error("useActiveAccount must be used within ActiveAccountProvider");
  return ctx;
}
