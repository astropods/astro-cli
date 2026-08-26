import { useCallback, useSyncExternalStore } from "react";
import {
  getPersistentStorageSnapshot,
  removePersistentStorageSnapshot,
  setPersistentStorageSnapshot,
  subscribePersistentStorage,
} from "@/lib/persistent-storage";

function storageKey(userId: string): string {
  return `astro:free-trial-modal:pending:${userId}`;
}

const PENDING = "true";

/** One-time, per-user flag for the free trial modal. usePersistentCoachmark
 *  inverted: presence means "show it". Set at account creation, cleared on
 *  first close. */
export function usePendingFreeTrialModal(userId?: string) {
  const key = userId ? storageKey(userId) : null;

  const subscribe = useCallback(
    (listener: () => void) => (key ? subscribePersistentStorage(key, listener) : () => undefined),
    [key],
  );
  const stored = useSyncExternalStore(
    subscribe,
    () => (key ? getPersistentStorageSnapshot(key) : null),
    () => null,
  );

  const markPending = useCallback(() => {
    if (key) setPersistentStorageSnapshot(key, PENDING);
  }, [key]);

  const clearPending = useCallback(() => {
    if (key) removePersistentStorageSnapshot(key);
  }, [key]);

  return { pending: stored === PENDING, markPending, clearPending };
}
