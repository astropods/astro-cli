import { useState } from "react";
import { useAuth } from "@/lib/auth";

export function useDefaultAccount() {
  const { accounts, personalAccount } = useAuth();

  const [storedDefault] = useState<string | null>(() => {
    try { return localStorage.getItem("astro:default-account"); } catch { return null; }
  });

  const validStoredDefault = storedDefault && accounts.some((a) => a.name === storedDefault) ? storedDefault : null;
  const defaultAccount = validStoredDefault ?? personalAccount?.name;

  return { defaultAccount, validStoredDefault };
}
