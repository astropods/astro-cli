import { useSyncExternalStore, useEffect, createContext, useContext } from "react";

export type Theme = "light" | "dark" | "auto";

/** The resolved theme used whenever no explicit choice has been made. */
export const DEFAULT_THEME: "light" | "dark" = "dark";

const STORAGE_KEY = "astro:theme";
const COOKIE_NAME = "astro-theme";
const COOKIE_MAX_AGE = 31536000; // 1 year

/**
 * Context provided by the Root component with the cookie-derived theme.
 * Used by useResolvedTheme() for its server snapshot so SSR renders
 * components with the correct theme (e.g. chart colors, starfield).
 */
export const ServerThemeContext = createContext<"light" | "dark">(DEFAULT_THEME);

function writeCookie(resolved: "light" | "dark") {
  if (typeof document === "undefined") return;
  document.cookie = `${COOKIE_NAME}=${resolved};path=/;max-age=${COOKIE_MAX_AGE};SameSite=Lax`;
}

export function parseCookieTheme(cookieHeader: string | null): "light" | "dark" {
  if (!cookieHeader) return DEFAULT_THEME;
  const match = cookieHeader.match(new RegExp(`(?:^|;\\s*)${COOKIE_NAME}=(light|dark)`));
  return (match?.[1] as "light" | "dark") ?? DEFAULT_THEME;
}

function getSystemTheme(): "light" | "dark" {
  if (typeof window === "undefined" || !window.matchMedia) return DEFAULT_THEME;
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
  return DEFAULT_THEME;
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
  const resolved = next === "auto" ? getSystemTheme() : next;
  writeCookie(resolved);
  snapshot = next;
  applyTheme(next);
  notify();
}

if (typeof window !== "undefined") {
  // Apply the resolved class on module init so `<html>` reflects the
  // persisted choice as soon as the bundle evaluates, before any component
  // mounts. Also write the cookie so subsequent SSR requests have it
  // (handles migration from localStorage-only).
  applyTheme(snapshot);
  writeCookie(snapshot === "auto" ? getSystemTheme() : snapshot);

  // Cross-tab sync: pick up writes from sibling tabs and rebroadcast.
  window.addEventListener("storage", (e) => {
    if (e.key !== STORAGE_KEY) return;
    snapshot = loadTheme();
    applyTheme(snapshot);
    writeCookie(snapshot === "auto" ? getSystemTheme() : snapshot);
    notify();
  });

  // System preference change while in `auto` mode.
  if (window.matchMedia) {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    mq.addEventListener("change", () => {
      if (snapshot === "auto") {
        applyTheme("auto");
        writeCookie(getSystemTheme());
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

function getResolvedTheme(): "light" | "dark" {
  const stored = loadTheme();
  return stored === "auto" ? getSystemTheme() : stored;
}

/** Returns the resolved theme ("light" | "dark"), tracking both user choice and system preference.
 *  SSR-safe: uses the cookie-derived theme from ServerThemeContext on the server. */
export function useResolvedTheme(): "light" | "dark" {
  const serverTheme = useContext(ServerThemeContext);
  return useSyncExternalStore(subscribe, getResolvedTheme, () => serverTheme);
}
