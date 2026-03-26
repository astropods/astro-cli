import { useEffect, useState } from "react";

export function useMediaBreakpoint(breakpoint: number) {
  const [matches, setMatches] = useState(() =>
    typeof window !== "undefined" ? window.innerWidth < breakpoint : false,
  );

  useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${breakpoint - 1}px)`);
    setMatches(mql.matches);
    const onChange = () => setMatches(mql.matches);
    mql.addEventListener("change", onChange);
    return () => mql.removeEventListener("change", onChange);
  }, [breakpoint]);

  return matches;
}

export function useCompactLayout() {
  return useMediaBreakpoint(1180);
}

export function useIsMobile() {
  return useMediaBreakpoint(768);
}
