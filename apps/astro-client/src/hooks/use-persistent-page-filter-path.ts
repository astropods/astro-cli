import { useCallback, useMemo, useSyncExternalStore } from "react";
import {
  getPersistentStorageSnapshot,
  subscribePersistentStorage,
} from "@/lib/persistent-storage";
import { useAuth } from "@/lib/auth";
import { canonicalizeUserResourceAccounts } from "@/lib/user-resource-scope";

/** Builds a navigation target from the destination page's persisted filters. */
export function usePersistentPageFilterPath(path: string, storageScope: string) {
  const { accounts } = useAuth();
  const memberships = useMemo(() => accounts.map((account) => account.name), [accounts]);
  const storageKey = `astro:page-filters:${storageScope}`;
  const subscribe = useCallback(
    (listener: () => void) => subscribePersistentStorage(storageKey, listener),
    [storageKey],
  );
  const getSnapshot = useCallback(
    () => getPersistentStorageSnapshot(storageKey),
    [storageKey],
  );
  const storedFilters = useSyncExternalStore(subscribe, getSnapshot, () => null);
  const validatedFilters = useMemo(() => {
    if (!storedFilters || memberships.length === 0) return "";
    const stored = new URLSearchParams(storedFilters);
    const requested = stored.getAll("account");
    if (requested.length === 0) {
      return stored.get("scope") === "all" ? "scope=all" : "";
    }

    const knownRequested = requested.filter((account) => memberships.includes(account));
    if (knownRequested.length === 0) return "";
    const selected = canonicalizeUserResourceAccounts(knownRequested, memberships);
    const validated = new URLSearchParams();
    if (selected.length === 0) {
      validated.set("scope", "all");
    } else {
      selected.forEach((account) => validated.append("account", account));
    }
    return validated.toString();
  }, [memberships, storedFilters]);

  return validatedFilters ? `${path}?${validatedFilters}` : path;
}
