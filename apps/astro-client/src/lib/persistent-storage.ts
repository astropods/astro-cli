type Listener = () => void;

const fallbackSnapshots = new Map<string, string | null>();
const listeners = new Map<string, Set<Listener>>();
const PAGE_FILTER_STORAGE_PREFIX = "astro:page-filters:";

function notify(storageKey: string) {
  for (const listener of listeners.get(storageKey) ?? []) listener();
}

export function getPersistentStorageSnapshot(
  storageKey: string,
): string | null {
  if (fallbackSnapshots.has(storageKey)) {
    return fallbackSnapshots.get(storageKey) ?? null;
  }
  if (typeof window === "undefined") return null;
  try {
    return localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}

export function subscribePersistentStorage(
  storageKey: string,
  listener: Listener,
) {
  const keyListeners = listeners.get(storageKey) ?? new Set<Listener>();
  keyListeners.add(listener);
  listeners.set(storageKey, keyListeners);
  return () => {
    keyListeners.delete(listener);
    if (keyListeners.size === 0) listeners.delete(storageKey);
  };
}

export function setPersistentStorageSnapshot(
  storageKey: string,
  value: string,
) {
  try {
    localStorage.setItem(storageKey, value);
    fallbackSnapshots.delete(storageKey);
  } catch {
    // Keep the in-memory value usable when storage is unavailable.
    fallbackSnapshots.set(storageKey, value);
  }
  notify(storageKey);
}

export function clearPageFilterStorage() {
  const keys = new Set(
    [...fallbackSnapshots.keys(), ...listeners.keys()].filter((key) =>
      key.startsWith(PAGE_FILTER_STORAGE_PREFIX),
    ),
  );
  let storageCleared = false;

  try {
    for (let index = 0; index < localStorage.length; index += 1) {
      const key = localStorage.key(index);
      if (key?.startsWith(PAGE_FILTER_STORAGE_PREFIX)) keys.add(key);
    }
    for (const key of keys) localStorage.removeItem(key);
    storageCleared = true;
  } catch {
    // Keep clearing in-memory values when storage is unavailable.
  }

  for (const key of keys) {
    if (storageCleared) fallbackSnapshots.delete(key);
    else fallbackSnapshots.set(key, null);
    notify(key);
  }
}

if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key === null) {
      fallbackSnapshots.clear();
      for (const key of listeners.keys()) notify(key);
      return;
    }

    fallbackSnapshots.delete(event.key);
    notify(event.key);
  });
}
