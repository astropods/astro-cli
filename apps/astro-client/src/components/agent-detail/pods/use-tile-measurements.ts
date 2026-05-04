import { useState, useCallback, useRef, useEffect } from "react";
import type { TileSize } from "./pod-layout";

/**
 * Measures rendered tile elements and collects their bounding sizes.
 *
 * Usage:
 * 1. Render tiles inside a hidden measurement container.
 * 2. Pass each tile's DOM element to `measureRef` via a callback ref.
 * 3. Once all tiles have been measured, `sizes` will be populated.
 */
export function useTileMeasurements(count: number) {
  const [sizes, setSizes] = useState<TileSize[] | null>(null);
  const collected = useRef<(TileSize | null)[]>([]);

  const measureRef = useCallback(
    (index: number) => (el: HTMLDivElement | null) => {
      if (!el) return;
      const { width, height } = el.getBoundingClientRect();
      collected.current[index] = { width: Math.ceil(width), height: Math.ceil(height) };

      // Check if all tiles have been measured.
      if (
        collected.current.length >= count &&
        collected.current.slice(0, count).every(Boolean)
      ) {
        setSizes(collected.current.slice(0, count) as TileSize[]);
      }
    },
    [count],
  );

  // Reset when count changes so stale positions are never used.
  useEffect(() => {
    collected.current = [];
    setSizes(null);
  }, [count]);

  return { sizes, measureRef };
}
