import { useSyncExternalStore, useEffect } from "react";

export type Theme = "light" | "dark" | "auto";

const STORAGE_KEY = "astro:theme";

function getSystemTheme(): "light" | "dark" {
  if (typeof window === "undefined" || !window.matchMedia) return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function applyTheme(theme: Theme) {
  if (typeof document === "undefined") return;
  const resolved = theme === "auto" ? getSystemTheme() : theme;
  document.documentElement.classList.toggle("dark", resolved === "dark");
}

function loadTheme(): Theme {
  try {
    const stored = typeof window === "undefined" ? null : localStorage.getItem(STORAGE_KEY);
    if (stored === "light" || stored === "dark" || stored === "auto") return stored;
  } catch {
    // localStorage may be unavailable.
  }
  return "light";
}

// Module-level store shared by every `useTheme()` consumer via
// `useSyncExternalStore`. A single snapshot + listener set means calling
// `setTheme` from one component re-renders all other consumers in the same
// tab.
const listeners = new Set<() => void>();
let snapshot: Theme = loadTheme();

function notify() {
  for (const listener of listeners) listener();
}

function subscribe(callback: () => void) {
  listeners.add(callback);
  return () => {
    listeners.delete(callback);
  };
}

function getSnapshot(): Theme {
  return snapshot;
}

export function setTheme(next: Theme) {
  if (snapshot === next) return;
  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // ignore
  }
  snapshot = next;
  applyTheme(next);
  notify();
}

if (typeof window !== "undefined") {
  // Apply the resolved class on module init so `<html>` reflects the
  // persisted choice as soon as the bundle evaluates, before any component
  // mounts.
  applyTheme(snapshot);

  // Cross-tab sync: pick up writes from sibling tabs and rebroadcast.
  window.addEventListener("storage", (e) => {
    if (e.key !== STORAGE_KEY) return;
    snapshot = loadTheme();
    applyTheme(snapshot);
    notify();
  });

  // System preference change while in `auto` mode.
  if (window.matchMedia) {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener("change", () => {
      if (snapshot === "auto") {
        applyTheme("auto");
        notify();
      }
    });
  }
}

export function useTheme() {
  const theme = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  // Keep `<html>` in sync with the current theme on every render.
  // `applyTheme` is idempotent so re-applying the same value is a no-op.
  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  return { theme, setTheme };
}
