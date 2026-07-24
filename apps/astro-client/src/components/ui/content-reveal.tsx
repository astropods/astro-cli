import { useLayoutEffect, useRef, useState } from "react";
import type { HTMLMotionProps } from "motion/react";
import { motion, useAnimationControls } from "motion/react";
import {
  type ResultSetRevealState,
  shouldRevealContent,
  useContentRevealMotion,
} from "./content-reveal-motion";

/**
 * The shared content entrance used by agent-detail tabs and as the motion
 * contract for filter-driven result surfaces.
 */
export function ContentReveal({
  children,
  ...props
}: HTMLMotionProps<"div">) {
  const revealMotion = useContentRevealMotion();

  return (
    <motion.div data-slot="content-reveal" {...props} {...revealMotion}>
      {children}
    </motion.div>
  );
}

interface ResultSetRevealProps extends HTMLMotionProps<"div"> {
  itemCount: number;
  settled: boolean;
  transitionKey?: string;
}

/**
 * Replays the shared reveal when initial results settle or a non-empty
 * destination selection replaces the current results. Later count changes
 * within the same settled selection remain static so polling stays quiet.
 */
export function ResultSetReveal({
  children,
  itemCount,
  settled,
  transitionKey,
  ...props
}: ResultSetRevealProps) {
  const controls = useAnimationControls();
  const revealMotion = useContentRevealMotion();
  const previousState = useRef<ResultSetRevealState | null>(null);

  useLayoutEffect(() => {
    const current = { itemCount, settled, transitionKey };
    const previous = previousState.current;
    previousState.current = current;
    controls.stop();

    if (!shouldRevealContent(previous, current)) {
      controls.set(revealMotion.animate);
      return;
    }

    if (revealMotion.initial !== false) {
      controls.set(revealMotion.initial);
    }
    void controls.start({
      ...revealMotion.animate,
      transition: revealMotion.transition,
    });
  }, [controls, itemCount, revealMotion, settled, transitionKey]);

  return (
    <motion.div
      data-slot="result-set-reveal"
      {...props}
      initial={
        !settled || itemCount === 0
          ? revealMotion.animate
          : revealMotion.initial
      }
      animate={controls}
    >
      {children}
    </motion.div>
  );
}

interface SettledContentRevealProps extends HTMLMotionProps<"div"> {
  transitionKey: string;
  settled: boolean;
}

/**
 * Keeps placeholder content static, then reveals the replacement once its
 * query settles. This is intended for account-scoped views that use
 * keepPreviousData rather than additive result lists.
 */
export function SettledContentReveal({
  children,
  transitionKey,
  settled,
  ...props
}: SettledContentRevealProps) {
  const [revealedKey, setRevealedKey] = useState<string | null>(() =>
    settled ? transitionKey : null,
  );

  useLayoutEffect(() => {
    if (!settled) return;
    setRevealedKey((current) =>
      current === transitionKey ? current : transitionKey,
    );
  }, [settled, transitionKey]);

  if (revealedKey === null) {
    return (
      <motion.div
        data-slot="settled-content-pending"
        {...props}
        initial={false}
      >
        {children}
      </motion.div>
    );
  }

  return (
    <ContentReveal
      key={revealedKey}
      data-slot="settled-content-reveal"
      {...props}
    >
      {children}
    </ContentReveal>
  );
}
