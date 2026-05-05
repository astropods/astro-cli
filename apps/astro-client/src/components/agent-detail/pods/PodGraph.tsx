import { useEffect, useRef, useMemo, type ReactNode } from "react";
import { motion, AnimatePresence } from "motion/react";
import { computePodPositions, type TileSize, type Position } from "./pod-layout";
import { computeMST } from "../starfield/constellation";
import { useTileMeasurements } from "./use-tile-measurements";
import { useContainerSize } from "@/hooks/use-container-size";

const VERTICAL_BREAKPOINT = 750;
const VERTICAL_GAP = 12;

function computeVerticalPositions(sizes: TileSize[]): Position[] {
  const totalHeight =
    sizes.reduce((sum, s) => sum + s.height, 0) + (sizes.length - 1) * VERTICAL_GAP;
  let y = -totalHeight / 2;
  return sizes.map((s) => {
    const pos = { x: 0, y: y + s.height / 2 };
    y += s.height + VERTICAL_GAP;
    return pos;
  });
}

interface PodGraphProps {
  count: number;
  renderTile: (index: number) => ReactNode;
  /** Effective visible width (container minus panel). Drives layout breakpoints. */
  effectiveWidth?: number;
}

/**
 * Layout-only component. Measures tiles, runs force-directed layout,
 * and positions them with spring animations. Draws constellation lines
 * between tiles via minimum spanning tree.
 *
 * Below 750px width, switches to a vertical stack layout.
 */
export function PodGraph({ count, renderTile, effectiveWidth }: PodGraphProps) {
  const { ref: containerRef, width: containerW, height: containerH } = useContainerSize();
  const { sizes, measureRef } = useTileMeasurements(count);
  const hasAnimatedIn = useRef(false);

  // Use effectiveWidth (accounts for panel) if provided, otherwise container width
  const layoutWidth = effectiveWidth ?? containerW;
  const isVertical = layoutWidth > 0 && layoutWidth < VERTICAL_BREAKPOINT;

  // Treat sizes as stale whenever it doesn't match the current count — the
  // useEffect inside useTileMeasurements that resets sizes runs after commit,
  // so a render that happens between "count changed" and "effect fired" would
  // otherwise lay out N old positions while renderTile(i) reads the new
  // (shorter) source array, dereferencing undefined.
  const sizesValid = sizes !== null && sizes.length === count;

  const positions = useMemo(() => {
    if (!sizesValid) return null;
    return isVertical ? computeVerticalPositions(sizes!) : computePodPositions(sizes!);
  }, [sizes, sizesValid, isVertical]);

  const graphPositions = useMemo(
    () => (sizesValid ? computePodPositions(sizes!) : null),
    [sizes, sizesValid],
  );

  const edges = useMemo(
    () => (graphPositions ? computeMST(graphPositions) : []),
    [graphPositions],
  );

  const centerX = containerW / 2;
  const centerY = containerH / 2;
  const measured = positions !== null;
  const ready = measured && containerW > 0;

  // Flip after the initial entrance animation completes
  useEffect(() => {
    if (ready && !hasAnimatedIn.current) {
      const id = setTimeout(() => { hasAnimatedIn.current = true; }, count * 40 + 600);
      return () => clearTimeout(id);
    }
  }, [ready, count]);

  return (
    <div ref={containerRef} className="relative h-full flex-1 overflow-hidden">
      {/* Pass 1: Hidden measurement render */}
      {!measured && (
        <div className="pointer-events-none invisible absolute left-0 top-0">
          {Array.from({ length: count }, (_, i) => (
            <div key={i} ref={measureRef(i)} className="inline-block">
              {renderTile(i)}
            </div>
          ))}
        </div>
      )}

      {/* Constellation lines — only in graph mode */}
      {ready && graphPositions && !isVertical && (
        <svg className="pointer-events-none absolute inset-0 h-full w-full">
          {edges.map((edge, i) => (
            <motion.line
              key={`${edge.from}-${edge.to}`}
              x1={centerX + graphPositions[edge.from].x}
              y1={centerY + graphPositions[edge.from].y}
              x2={centerX + graphPositions[edge.to].x}
              y2={centerY + graphPositions[edge.to].y}
              className="stroke-foreground/10"
              strokeWidth={1}
              initial={{ pathLength: 0, opacity: 0 }}
              animate={{ pathLength: 1, opacity: 1 }}
              transition={{ duration: 0.5, delay: 0.3 + i * 0.06, ease: "easeOut" }}
            />
          ))}
        </svg>
      )}

      {/* Positioned tiles */}
      <AnimatePresence>
        {ready &&
          positions!.map((pos, i) => (
            <motion.div
              key={i}
              className="absolute -translate-x-1/2 -translate-y-1/2"
              initial={hasAnimatedIn.current ? false : { opacity: 0, scale: 0.8, left: centerX, top: centerY }}
              animate={{
                opacity: 1,
                scale: 1,
                left: centerX + pos.x,
                top: centerY + pos.y,
              }}
              exit={{ opacity: 0, scale: 0.8 }}
              transition={
                hasAnimatedIn.current
                  ? { type: "spring", bounce: 0.1, duration: 0.4 }
                  : { type: "spring", bounce: 0.15, duration: 0.6, delay: i * 0.04 }
              }
            >
              {renderTile(i)}
            </motion.div>
          ))}
      </AnimatePresence>
    </div>
  );
}
