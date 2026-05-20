import { useEffect, useState } from "react";
import { useNavigation, useRevalidator } from "react-router";
import { motion } from "motion/react";

// Thin progress bar pinned to the top of the viewport that surfaces React
// Router navigation / revalidation activity. Tied to nav + revalidator only —
// NOT useIsFetching. Account-scoped queries with `placeholderData:
// keepPreviousData` keep old data on screen across key flips, so the loader
// signal is the right proxy for "the new page is ready." Watching every query
// causes the bar to get stuck whenever polling is active (deployment status,
// knowledge transitional states all poll every 3s).
export function NavigationProgressBar() {
  const navigation = useNavigation();
  const revalidator = useRevalidator();
  const active = navigation.state !== "idle" || revalidator.state !== "idle";

  // Keep the bar mounted briefly after the loader resolves so the sweep
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
