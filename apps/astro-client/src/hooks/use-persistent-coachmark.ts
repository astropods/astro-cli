import { useCallback, useSyncExternalStore } from "react";
import {
  getPersistentStorageSnapshot,
  setPersistentStorageSnapshot,
  subscribePersistentStorage,
} from "@/lib/persistent-storage";

export function coachmarkStorageKey(
  coachmarkId: string,
  userId: string,
): string {
  return `astro:onboarding:${coachmarkId}:${userId}`;
}

const DISMISSED = "true";

/** Persists a one-time coachmark dismissal independently for each user. */
export function usePersistentCoachmark(coachmarkId: string, userId?: string) {
  const storageKey = userId ? coachmarkStorageKey(coachmarkId, userId) : null;

  const subscribe = useCallback(
    (listener: () => void) =>
      storageKey
        ? subscribePersistentStorage(storageKey, listener)
        : () => undefined,
    [storageKey],
  );
  const stored = useSyncExternalStore(
    subscribe,
    () => (storageKey ? getPersistentStorageSnapshot(storageKey) : null),
    () => DISMISSED,
  );

  const dismiss = useCallback(() => {
    if (storageKey) setPersistentStorageSnapshot(storageKey, DISMISSED);
  }, [storageKey]);

  return { dismissed: !storageKey || stored === DISMISSED, dismiss };
}
