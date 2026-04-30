import { createContext, useContext, useState, useCallback, type ReactNode } from "react";
import { useAuth } from "@/lib/auth";

interface ActiveAccountContextValue {
  activeAccount: string;
  setActiveAccount: (account: string) => void;
}

const ActiveAccountContext = createContext<ActiveAccountContextValue | null>(null);

export function ActiveAccountProvider({ children }: { children: ReactNode }) {
  const { accounts, personalAccount } = useAuth();

  const [storedDefault, setStoredDefault] = useState<string | null>(() => {
    try { return localStorage.getItem("astro:default-account"); } catch { return null; }
  });

  const validStoredDefault =
    storedDefault && accounts.some((a) => a.name === storedDefault) ? storedDefault : null;
  const activeAccount = validStoredDefault || personalAccount?.name || "";

  const setActiveAccount = useCallback((accountName: string) => {
    if (accountName === personalAccount?.name) {
      localStorage.removeItem("astro:default-account");
      setStoredDefault(null);
    } else {
      localStorage.setItem("astro:default-account", accountName);
      setStoredDefault(accountName);
    }
  }, [personalAccount?.name]);

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
