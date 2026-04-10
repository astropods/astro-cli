import { useSyncExternalStore } from "react";

/**
 * Session-local avatar override store.
 *
 * After an avatar upload, call `bustAvatar(handle, blob)` to immediately show
 * the new image in every `<UserAvatar>` for that handle — no network request
 * needed. The blob URL is purely local and only lives for the current browser
 * session. On next page load the CDN's standard Cache-Control / ETag headers
 * take over.
 */

const overrides = new Map<string, string>();
const listeners = new Set<() => void>();

function emit() {
  for (const fn of listeners) fn();
}

/** Override the avatar for `handle` with a local blob URL for instant feedback. */
export function bustAvatar(handle: string, blob: Blob): void {
  const prev = overrides.get(handle);
  if (prev) URL.revokeObjectURL(prev);
  overrides.set(handle, URL.createObjectURL(blob));
  emit();
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
