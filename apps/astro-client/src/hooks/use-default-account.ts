import { useState } from "react";
import { useAuth } from "@/lib/auth";

export function useDefaultAccount() {
  const { accounts, personalAccount } = useAuth();

  const [storedDefault, setStoredDefault] = useState<string | null>(() => {
    try { return localStorage.getItem("astro:default-account"); } catch { return null; }
  });

  const validStoredDefault = storedDefault && accounts.some((a) => a.name === storedDefault) ? storedDefault : null;
  const defaultAccount = validStoredDefault ?? personalAccount?.name;

  const handleSetDefault = (accountName: string) => {
    const isDefault = accountName === defaultAccount;
    if (isDefault) {
      if (accountName !== personalAccount?.name) {
        localStorage.removeItem("astro:default-account");
        setStoredDefault(null);
      }
    } else if (accountName === personalAccount?.name) {
      localStorage.removeItem("astro:default-account");
      setStoredDefault(null);
    } else {
      localStorage.setItem("astro:default-account", accountName);
      setStoredDefault(accountName);
    }
  };

  return { defaultAccount, validStoredDefault, handleSetDefault };
}
