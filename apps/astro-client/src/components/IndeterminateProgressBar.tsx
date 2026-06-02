import { useEffect, useLayoutEffect, useRef, useState, type TransitionEvent } from "react";

// nprogress-style trickle on `active` (snap-in → ease toward ceiling → snap-to-done).
// Design rationale (why not useIsFetching) lives in the PR changelog.
const INITIAL_PROGRESS = 15;
const TRICKLE_CEILING = 90;
const TRICKLE_INTERVAL_MS = 200;
const TRICKLE_DECAY = 0.05;
const FINISH_MS = 300;

export function IndeterminateProgressBar({ active }: { active: boolean }) {
  const [visible, setVisible] = useState(false);
  const [progress, setProgress] = useState(0);
  // `finishing` means we've snapped to 100 and are fading out; unmount is
  // tied to the actual opacity transition end below, not a parallel timer.
  const [finishing, setFinishing] = useState(false);

  const trickleRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useLayoutEffect(() => {
    if (active) {
      setVisible(true);
      setFinishing(false);
      setProgress(INITIAL_PROGRESS);
    }
  }, [active]);

  useEffect(() => {
    if (active) {
      trickleRef.current = setInterval(() => {
        setProgress((p) => (p < TRICKLE_CEILING ? p + (TRICKLE_CEILING - p) * TRICKLE_DECAY : p));
      }, TRICKLE_INTERVAL_MS);
    } else if (visible) {
      if (trickleRef.current) {
        clearInterval(trickleRef.current);
        trickleRef.current = null;
      }
      setProgress(100);
      setFinishing(true);
    }
    return () => {
      if (trickleRef.current) clearInterval(trickleRef.current);
    };
  }, [active, visible]);

  // Unmount when the opacity fade actually completes. Filters out the inner
  // bar's `width` transition-end events that bubble up to the wrapper, and
  // skips spurious fires when `finishing` was reset by a fast re-activation.
  function handleTransitionEnd(e: TransitionEvent<HTMLDivElement>) {
    if (!finishing || e.propertyName !== "opacity") return;
    setVisible(false);
    setProgress(0);
    setFinishing(false);
  }

  if (!active && !visible) return null;

  const barWidth = active ? Math.max(progress, INITIAL_PROGRESS) : progress;

  return (
    <div
      aria-hidden
      onTransitionEnd={handleTransitionEnd}
      className="pointer-events-none fixed inset-x-0 top-0 z-[9999] h-0.5 overflow-hidden"
      style={{
        opacity: finishing ? 0 : 1,
        transition: `opacity ${FINISH_MS}ms ease`,
      }}
    >
      <div
        className="h-full bg-primary"
        style={{
          width: `${barWidth}%`,
          transition: "width 200ms ease-out",
        }}
      />
    </div>
  );
}
