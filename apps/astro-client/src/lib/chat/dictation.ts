import { useSyncExternalStore } from "react";
import {
  WebSpeechDictationAdapter,
  type DictationState,
} from "@assistant-ui/react";

/**
 * Whether browser-native dictation (Web Speech API) is usable in this session.
 *
 * Single source of truth so adapter registration (the chat runtime provider)
 * and mic-button visibility (the composer) can never diverge. Computed once and
 * guarded for SSR — `isSupported()` reads `window`, so we don't call it during
 * a server render (it would return false there anyway, but we don't want to
 * depend on the library's internal guard).
 *
 * Do NOT read this directly in render — use `useDictationSupported()` so the
 * server and first client render agree (see below).
 */
export const DICTATION_SUPPORTED =
  typeof window !== "undefined" && WebSpeechDictationAdapter.isSupported();

/**
 * Process-wide dictation adapter, created once when supported. The adapter is
 * stateless across uses (each `listen()` opens its own session), so a single
 * instance is safe to share and gives the chat runtime a stable identity
 * without a per-render `useMemo`. `undefined` when unsupported (e.g. SSR).
 *
 * The composer no longer shows the live transcript as text — it shows an
 * audio-reactive waveform while listening (see DictationWaveform) and the
 * finalized words land in the input when dictation stops. So we register the
 * library adapter as-is; no interim-caption wrapper.
 *
 * Assumes at most one composer dictates at a time — true today (chat is a single
 * side panel). If multiple chat composers can ever be mounted and dictate
 * simultaneously, give each runtime its own adapter instance at that point.
 */
export const dictationAdapter = DICTATION_SUPPORTED
  ? new WebSpeechDictationAdapter()
  : undefined;

// Support is fixed for the session — nothing ever changes — so subscribe is a
// no-op and the snapshots are constant.
const noopSubscribe = () => () => {};
const getSupportedSnapshot = () => DICTATION_SUPPORTED;
const getServerSnapshot = () => false;

/**
 * SSR-safe view of {@link DICTATION_SUPPORTED} for use in render. Returns
 * `false` for the server snapshot so the server HTML and the first client
 * (hydration) render agree — then settles to the real value on the client.
 * This avoids a hydration mismatch on the mic button if the chat subtree is
 * ever server-rendered (e.g. via prefetch/dehydration).
 */
export function useDictationSupported(): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    getSupportedSnapshot,
    getServerSnapshot,
  );
}

/**
 * Whether a dictation session is currently capturing. The adapter reports a
 * status for every phase except completion; `"ended"` is the only terminal
 * state, so anything else (including `"error"`) counts as still-active for UI
 * purposes (the mic is shown as recording and the next click stops it).
 */
export function isDictationActive(
  dictation: DictationState | undefined,
): boolean {
  return dictation !== undefined && dictation.status.type !== "ended";
}
