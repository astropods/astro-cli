import { useEffect, useRef, useState } from "react";

/**
 * Track the rendered width and height of a DOM element via ResizeObserver.
 * Returns a ref to attach and the rounded pixel dimensions.
 */
export function useContainerSize<T extends HTMLElement = HTMLDivElement>() {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(0);
  const [height, setHeight] = useState(0);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => {
      const rect = entry.contentRect;
      setWidth(Math.round(rect.width));
      setHeight(Math.round(rect.height));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  return { ref, width, height };
}
