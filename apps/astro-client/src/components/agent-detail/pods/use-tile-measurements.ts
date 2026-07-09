import { useCallback, useEffect, useRef, useState } from "react";
import type { TileSize } from "./pod-layout";

/**
 * Continuously measures each tile's border box with a single `ResizeObserver`,
 * so a tile that grows later (an async error row, a status flip) re-drives the
 * layout. Sizes come from `borderBoxSize`, which ignores the transforms tiles
 * are positioned with, so animating a tile never feeds back into the layout.
 * Attach `measureRef(i)` to each tile; `sizes[i]` is null until first measured.
 */
export function useTileMeasurements(count: number) {
  const [sizes, setSizes] = useState<(TileSize | null)[]>(() =>
    Array.from({ length: count }, () => null),
  );
  const sizesRef = useRef<(TileSize | null)[]>(sizes);
  const indexToEl = useRef<Map<number, Element>>(new Map());
  const elToIndex = useRef<Map<Element, number>>(new Map());
  const refCache = useRef<Map<number, (el: HTMLElement | null) => void>>(new Map());
  const observerRef = useRef<ResizeObserver | null>(null);

  const commit = useCallback((index: number, width: number, height: number) => {
    const prev = sizesRef.current[index];
    if (prev && prev.width === width && prev.height === height) return;
    const next = sizesRef.current.slice();
    next[index] = { width, height };
    sizesRef.current = next;
    setSizes(next);
  }, []);

  const getObserver = useCallback(() => {
    if (!observerRef.current) {
      observerRef.current = new ResizeObserver((entries) => {
        for (const entry of entries) {
          const index = elToIndex.current.get(entry.target);
          if (index === undefined) continue;
          const box = entry.borderBoxSize?.[0];
          const width = Math.ceil(box ? box.inlineSize : entry.contentRect.width);
          const height = Math.ceil(box ? box.blockSize : entry.contentRect.height);
          commit(index, width, height);
        }
      });
    }
    return observerRef.current;
  }, [commit]);

  // Cache one ref per index so the element isn't re-observed on every render.
  const measureRef = useCallback(
    (index: number) => {
      let fn = refCache.current.get(index);
      if (!fn) {
        fn = (el: HTMLElement | null) => {
          const observer = getObserver();
          const prevEl = indexToEl.current.get(index);
          if (prevEl && prevEl !== el) {
            observer.unobserve(prevEl);
            elToIndex.current.delete(prevEl);
            indexToEl.current.delete(index);
          }
          if (el) {
            indexToEl.current.set(index, el);
            elToIndex.current.set(el, index);
            observer.observe(el);
            // Measure now so the first layout doesn't wait a frame for the observer.
            const rect = el.getBoundingClientRect();
            commit(index, Math.ceil(rect.width), Math.ceil(rect.height));
          }
        };
        refCache.current.set(index, fn);
      }
      return fn;
    },
    [getObserver, commit],
  );

  // Resize the sizes array to count (keeping survivors) and release dropped tiles.
  useEffect(() => {
    if (sizesRef.current.length !== count) {
      const next = Array.from({ length: count }, (_, i) => sizesRef.current[i] ?? null);
      sizesRef.current = next;
      setSizes(next);
    }
    for (const [index, el] of indexToEl.current) {
      if (index >= count) {
        observerRef.current?.unobserve(el);
        elToIndex.current.delete(el);
        indexToEl.current.delete(index);
        refCache.current.delete(index);
      }
    }
  }, [count]);

  useEffect(() => () => observerRef.current?.disconnect(), []);

  return { sizes, measureRef };
}
