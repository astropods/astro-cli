import type { TargetAndTransition, Transition } from "motion/react";
import { useReducedMotion } from "motion/react";

interface ContentRevealMotion {
  initial: false | TargetAndTransition;
  animate: TargetAndTransition;
  transition: Transition;
}

const contentRevealMotion: ContentRevealMotion = {
  initial: { opacity: 0, y: 12 },
  animate: { opacity: 1, y: 0 },
  transition: { duration: 0.25, ease: "easeOut" },
};

const reducedContentRevealMotion: ContentRevealMotion = {
  initial: false,
  animate: { opacity: 1, y: 0 },
  transition: { duration: 0 },
};

export interface ResultSetRevealState {
  itemCount: number;
  settled: boolean;
  transitionKey?: string;
}

export function shouldRevealContent(
  previous: ResultSetRevealState | null,
  current: ResultSetRevealState,
) {
  if (!current.settled || current.itemCount === 0) return false;
  if (previous === null) return true;

  return (
    !previous.settled ||
    (current.transitionKey !== undefined &&
      current.transitionKey !== previous.transitionKey)
  );
}

export function useContentRevealMotion(): ContentRevealMotion {
  const reducedMotion = useReducedMotion();
  return reducedMotion ? reducedContentRevealMotion : contentRevealMotion;
}
