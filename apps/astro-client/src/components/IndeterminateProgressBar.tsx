import { useEffect, useState } from "react";
import { motion } from "motion/react";

/** Top-of-viewport indeterminate sweep used for navigation and in-page refetches. */
export function IndeterminateProgressBar({ active }: { active: boolean }) {
  // Keep the bar mounted briefly after activity ends so the sweep
  // completes visibly instead of disappearing mid-animation.
  const [show, setShow] = useState(false);
  useEffect(() => {
    if (active) {
      setShow(true);
      return;
    }
    const t = setTimeout(() => setShow(false), 250);
    return () => clearTimeout(t);
  }, [active]);

  if (!show) return null;

  return (
    <div
      aria-hidden
      className="pointer-events-none fixed inset-x-0 top-0 z-[9999] h-0.5 overflow-hidden"
    >
      <motion.div
        className="h-full w-1/3 bg-gradient-to-r from-transparent via-primary to-transparent"
        initial={{ x: "-100%" }}
        animate={{ x: "400%" }}
        transition={{ duration: 1.1, repeat: Infinity, ease: "easeInOut" }}
      />
    </div>
  );
}
