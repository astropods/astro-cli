import { useEffect, useState } from "react";

/**
 * Matches viewport width. Initial render is always false (SSR + first client paint)
 * so markup does not depend on window size until after mount.
 */
export function useMediaBreakpoint(breakpoint: number) {
  const [matches, setMatches] = useState(false);

  useEffect(() => {
    const query = `(max-width: ${breakpoint - 1}px)`;
    const mql = window.matchMedia(query);
    const update = () => setMatches(mql.matches);
    update();
    mql.addEventListener("change", update);
    return () => mql.removeEventListener("change", update);
  }, [breakpoint]);

  return matches;
}

export function useCompactLayout() {
  return useMediaBreakpoint(1180);
}

export function useIsMobile() {
  return useMediaBreakpoint(768);
}
