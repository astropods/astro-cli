import { useEffect, useState } from "react";

// A deploy that stays in the "deploying" state past this long is treated as
// stuck. This is the defensive backstop for a hang with no clear event; the
// primary detection is event-driven (the server surfaces the blocking event).
export const STUCK_DEPLOY_AFTER_MS = 5 * 60 * 1000;

/**
 * Reports true once the deploy has been in the deploying state for `afterMs`,
 * measured from the server-provided `startedAt` (the status_changed_at
 * timestamp) rather than component mount, so it reflects real deploy age and
 * survives reloads and revisits. Schedules a single timer for the remaining
 * time so the banner appears without waiting for the next poll, and resets when
 * the deploy ends or the start timestamp changes.
 */
export function useDeployStuckByAge(
  startedAt: string | undefined,
  isDeploying: boolean,
  afterMs: number = STUCK_DEPLOY_AFTER_MS,
): boolean {
  const [stuck, setStuck] = useState(false);
  useEffect(() => {
    if (!isDeploying || !startedAt) {
      setStuck(false);
      return;
    }
    const start = Date.parse(startedAt);
    if (Number.isNaN(start)) {
      setStuck(false);
      return;
    }
    const elapsed = Date.now() - start;
    if (elapsed >= afterMs) {
      setStuck(true);
      return;
    }
    setStuck(false);
    const timer = setTimeout(() => setStuck(true), afterMs - elapsed);
    return () => clearTimeout(timer);
  }, [startedAt, isDeploying, afterMs]);
  return stuck;
}
