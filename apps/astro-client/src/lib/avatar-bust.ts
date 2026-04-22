import { useSyncExternalStore } from "react";

/**
 * Session-local avatar override store.
 *
 * After an avatar upload, call `bustAvatar(handle, blob)` to immediately show
 * the new image in every `<UserAvatar>` for that handle — no network request
 * needed. The blob URL is purely local and only lives for the current browser
 * session. On next page load the CDN's standard Cache-Control / ETag headers
 * take over.
 *
 * Agent avatar busts are also persisted to sessionStorage so they survive page
 * refreshes within the same tab. This matters because: (a) in local dev the CDN
 * URL always 404s (no VITE_ASSETS_URL), and (b) the user may have picked a preset
 * SVG that differs from the deterministic `account/name` fallback.
 */

const SESSION_PREFIX = "astro:agent-avatar:";

const overrides = new Map<string, string>();
const listeners = new Set<() => void>();

function emit() {
  for (const fn of listeners) fn();
}

// On module init, restore any persisted agent avatar SVGs from sessionStorage.
// Converts stored SVG strings back to data URLs so they work as <img> src values.
// Only restores entries that look like valid SVG (starts with "<svg" or "<SVG").
try {
  const keysToRemove: string[] = [];
  for (let i = 0; i < sessionStorage.length; i++) {
    const key = sessionStorage.key(i);
    if (key?.startsWith(SESSION_PREFIX)) {
      const svg = sessionStorage.getItem(key);
      if (svg && svg.trimStart().toLowerCase().startsWith("<svg")) {
        overrides.set(key.slice(SESSION_PREFIX.length), `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`);
      } else {
        // Stale or invalid entry (e.g. PNG binary data from old code) — clean it up.
        keysToRemove.push(key);
      }
    }
  }
  for (const key of keysToRemove) sessionStorage.removeItem(key);
} catch { /* sessionStorage unavailable */ }

/** Override the avatar for `handle` with a local blob URL for instant feedback. */
export function bustAvatar(handle: string, blob: Blob): void {
  const prev = overrides.get(handle);
  if (prev?.startsWith("blob:")) URL.revokeObjectURL(prev);
  overrides.set(handle, URL.createObjectURL(blob));
  emit();
}

/**
 * Override the avatar for an agent blueprint.
 * Stores a blob URL in memory for immediate display, and persists the raw SVG
 * string to sessionStorage so the override survives page refreshes.
 */
export function bustAgentAvatar(account: string, name: string, blob: Blob): void {
  const key = `agent:${account}/${name}`;
  const prev = overrides.get(key);
  if (prev?.startsWith("blob:")) URL.revokeObjectURL(prev);
  overrides.set(key, URL.createObjectURL(blob));
  emit();

  // Only persist SVG blobs to sessionStorage — PNG/JPEG blobs are already
  // stored on the server, so a fresh load will fetch the CDN URL instead.
  if (blob.type === "image/svg+xml") {
    blob.text().then((svg) => {
      try {
        sessionStorage.setItem(`${SESSION_PREFIX}${key}`, svg);
      } catch { /* quota exceeded or sessionStorage unavailable */ }
    });
  }
}

/** React hook — returns a local URL override for an agent avatar, or `undefined`. */
export function useAgentAvatarBust(account: string, name: string): string | undefined {
  return useAvatarBust(`agent:${account}/${name}`);
}

/** Override the avatar for a deployment with a local blob URL for instant feedback. */
export function bustDeploymentAvatar(id: string, blob: Blob): void {
  const key = `deployment:${id}`;
  const prev = overrides.get(key);
  if (prev?.startsWith("blob:")) URL.revokeObjectURL(prev);
  overrides.set(key, URL.createObjectURL(blob));
  emit();
}

/** React hook — returns a local blob URL override for a deployment avatar, or `undefined`. */
export function useDeploymentAvatarBust(id: string): string | undefined {
  return useAvatarBust(`deployment:${id}`);
}

/** React hook — returns a local blob URL override for `handle`, or `undefined`. */
export function useAvatarBust(handle: string): string | undefined {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => overrides.get(handle),
    () => undefined,
  );
}
