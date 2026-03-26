import * as React from "react";

const DEFAULT_BREAKPOINT = 1180;

export function useCompactLayout(breakpoint = DEFAULT_BREAKPOINT) {
  const [isCompact, setIsCompact] = React.useState<boolean | undefined>(undefined);

  React.useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${breakpoint - 1}px)`);
    const onChange = () => {
      setIsCompact(window.innerWidth < breakpoint);
    };
    mql.addEventListener("change", onChange);
    setIsCompact(window.innerWidth < breakpoint);
    return () => mql.removeEventListener("change", onChange);
  }, [breakpoint]);

  return !!isCompact;
}
