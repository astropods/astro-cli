import { createContext, useContext, useState, type ReactNode } from "react";
import { useAuth } from "@/lib/auth";

interface ActiveAccountContextValue {
  activeAccount: string;
  defaultAccount: string | undefined;
  setActiveAccount: (account: string) => void;
  toggleDefault: (account: string) => void;
}

const ActiveAccountContext = createContext<ActiveAccountContextValue | null>(null);

export function ActiveAccountProvider({ children }: { children: ReactNode }) {
  const { accounts, personalAccount } = useAuth();

  const [storedDefault, setStoredDefault] = useState<string | null>(() => {
    try { return localStorage.getItem("astro:default-account"); } catch { return null; }
  });

  const validStoredDefault =
    storedDefault && accounts.some((a) => a.name === storedDefault) ? storedDefault : null;
  const defaultAccount = validStoredDefault ?? personalAccount?.name;
  const activeAccount = validStoredDefault || personalAccount?.name || "";

  const setActiveAccount = (accountName: string) => {
    if (accountName === personalAccount?.name) {
      localStorage.removeItem("astro:default-account");
      setStoredDefault(null);
    } else {
      localStorage.setItem("astro:default-account", accountName);
      setStoredDefault(accountName);
    }
  };

  const toggleDefault = (accountName: string) => {
    const isCurrentDefault = accountName === defaultAccount;
    if (isCurrentDefault && accountName !== personalAccount?.name) {
      localStorage.removeItem("astro:default-account");
      setStoredDefault(null);
    } else if (accountName === personalAccount?.name) {
      localStorage.removeItem("astro:default-account");
      setStoredDefault(null);
    } else {
      localStorage.setItem("astro:default-account", accountName);
      setStoredDefault(accountName);
    }
  };

  return (
    <ActiveAccountContext.Provider value={{ activeAccount, defaultAccount, setActiveAccount, toggleDefault }}>
      {children}
    </ActiveAccountContext.Provider>
  );
}

export function useActiveAccount() {
  const ctx = useContext(ActiveAccountContext);
  if (!ctx) throw new Error("useActiveAccount must be used within ActiveAccountProvider");
  return ctx;
}
