import { useSyncExternalStore } from "react";

export interface Experiments {
  evals: boolean;
  /**
   * Serve the Insights page from the rollup-backed `/api/v2` endpoint instead
   * of `/api/v1`. Both are live and wire-compatible, so this exists to compare
   * them on the same account without redeploying.
   */
  insightsRollups: boolean;
}

const STORAGE_KEY = "astro:experiments";

const DEFAULTS: Experiments = {
  evals: false,
  insightsRollups: false,
};

export const hasExperiments = Object.keys(DEFAULTS).length > 0;

function load(): Experiments {
  try {
    const raw = typeof window === "undefined" ? null : localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...DEFAULTS };
    return { ...DEFAULTS, ...JSON.parse(raw) };
  } catch {
    return { ...DEFAULTS };
  }
}

function save(experiments: Experiments) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(experiments));
  } catch {
    // localStorage may be unavailable (SSR, private mode); silently ignore.
  }
}

// Module-level store shared by every `useExperiments()` consumer via
// `useSyncExternalStore`. A single snapshot + listener set means toggling
// from one component re-renders all other consumers in the same tab.
//
// The `storage` event fires only in OTHER tabs (browsers do not dispatch it
// in the tab that wrote the value), so same-tab notification has to be
// driven manually by `notify()` after each `save()`.
const listeners = new Set<() => void>();
let snapshot: Experiments = load();

function notify() {
  for (const listener of listeners) listener();
}

function subscribe(callback: () => void) {
  listeners.add(callback);
  return () => {
    listeners.delete(callback);
  };
}

function getSnapshot(): Experiments {
  return snapshot;
}

export function setExperiment<K extends keyof Experiments>(key: K, value: Experiments[K]) {
  if (snapshot[key] === value) return;
  snapshot = { ...snapshot, [key]: value };
  save(snapshot);
  notify();
}

if (typeof window !== "undefined") {
  // Cross-tab sync: pick up writes from sibling tabs and rebroadcast to
  // local subscribers.
  window.addEventListener("storage", (e) => {
    if (e.key !== STORAGE_KEY) return;
    snapshot = load();
    notify();
  });
}

export function useExperiments() {
  const experiments = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return { experiments, setExperiment };
}
