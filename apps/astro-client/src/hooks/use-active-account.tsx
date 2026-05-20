import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { useRevalidator, useRouteLoaderData } from "react-router";
import { useAuth } from "@/lib/auth";
import {
  ACTIVE_ACCOUNT_COOKIE,
  LEGACY_ACTIVE_ACCOUNT_STORAGE_KEY,
  readCookieValue,
} from "@/lib/active-account";

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

function readActiveAccountCookie(): string | null {
  if (typeof document === "undefined") return null;
  return readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE);
}

export function ActiveAccountProvider({ children }: { children: ReactNode }) {
  const { accounts, personalAccount } = useAuth();
  const revalidator = useRevalidator();
  // Source initial value from the root loader (cookie-derived) rather than
  // localStorage so SSR and first client render agree — prevents a hydration
  // flash on the switcher.
  const rootData = useRouteLoaderData("root") as { activeAccount?: string } | undefined;
  const ssrAccount = rootData?.activeAccount ?? "";

  // Client override makes setActiveAccount feel instant; the revalidator
  // refreshes loader data underneath.
  const [override, setOverride] = useState<string | null>(null);
  const validOverride = override && accounts.some((a) => a.name === override) ? override : null;
  const activeAccount = validOverride || ssrAccount || personalAccount?.name || "";

  const setActiveAccount = useCallback((accountName: string) => {
    if (accountName === personalAccount?.name) {
      clearActiveAccountCookie();
      try { localStorage.removeItem(LEGACY_ACTIVE_ACCOUNT_STORAGE_KEY); } catch { /* ignore */ }
    } else {
      writeActiveAccountCookie(accountName);
      try { localStorage.setItem(LEGACY_ACTIVE_ACCOUNT_STORAGE_KEY, accountName); } catch { /* ignore */ }
    }
    setOverride(accountName);
    // Re-runs page loaders (and the root loader thanks to shouldRevalidate)
    // under the new cookie so SSR-backed data refreshes for the new org.
    revalidator.revalidate();
  }, [personalAccount?.name, revalidator]);

  // One-time migration for users from before the cookie existed: if
  // localStorage has a valid stored account but the cookie isn't set yet,
  // sync the cookie and revalidate so subsequent renders match.
  useEffect(() => {
    if (accounts.length === 0) return;
    // readCookieValue can throw URIError on malformed percent-encoding —
    // swallow it so a stale/broken cookie can't break the migration effect.
    let existing: string | null = null;
    try { existing = readActiveAccountCookie(); } catch { /* malformed cookie — treat as absent */ }
    if (existing) return;
    let stored: string | null = null;
    try { stored = localStorage.getItem(LEGACY_ACTIVE_ACCOUNT_STORAGE_KEY); } catch { /* ignore */ }
    if (stored && accounts.some((a) => a.name === stored)) {
      writeActiveAccountCookie(stored);
      revalidator.revalidate();
    }
  }, [accounts, revalidator]);

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
