import { useEffect, useRef, useMemo, useCallback, type ReactNode } from "react";
import { motion, AnimatePresence } from "motion/react";
import { computeColumnLayout, computeVerticalLayout, type LayoutTile, type Position } from "./pod-layout";
import { computeRelationshipEdges } from "./pod-edges";
import { classify, type Role } from "./classify";
import { useTileMeasurements } from "./use-tile-measurements";
import { useContainerSize } from "@/hooks/use-container-size";

const VERTICAL_BREAKPOINT = 750;

interface PodGraphProps {
  count: number;
  renderTile: (index: number) => ReactNode;
  /** Component label per tile, in index order — classifies each tile's role. */
  components?: string[];
  /** k8s kind per tile, in index order — fallback for role classification. */
  kinds?: string[];
  /** Stable id per tile, in index order, for animation identity. Falls back to the index. */
  keys?: string[];
  /** Effective visible width (container minus panel). Drives the layout breakpoint. */
  effectiveWidth?: number;
}

/**
 * Layout-only component. Measures tiles continuously ({@link useTileMeasurements}),
 * places them in deterministic role columns ({@link computeColumnLayout}), and
 * draws relationship edges ({@link computeRelationshipEdges}). Collapses to a
 * single vertical stack below {@link VERTICAL_BREAKPOINT}.
 */
export function PodGraph({ count, renderTile, components, kinds, keys, effectiveWidth }: PodGraphProps) {
  const { ref: containerRef, width: containerW, height: containerH } = useContainerSize();
  const { sizes, measureRef } = useTileMeasurements(count);
  const hasAnimatedIn = useRef(false);
  const keyOf = useCallback((i: number) => keys?.[i] ?? String(i), [keys]);

  const roles = useMemo<Role[]>(
    () => Array.from({ length: count }, (_, i) => classify(components?.[i], kinds?.[i])),
    [components, kinds, count],
  );

  const layoutWidth = effectiveWidth ?? containerW;
  const isVertical = layoutWidth > 0 && layoutWidth < VERTICAL_BREAKPOINT;

  // Wait until every tile for the current count is measured; the length check
  // rejects a stale render between a count change and the next measure.
  const sizesValid = sizes.length === count && sizes.every(Boolean);

  const layoutTiles = useMemo<LayoutTile[] | null>(
    () => (sizesValid ? roles.map((role, i) => ({ role, size: sizes[i]! })) : null),
    [sizesValid, roles, sizes],
  );

  const positions = useMemo<Position[] | null>(() => {
    if (!layoutTiles) return null;
    return isVertical ? computeVerticalLayout(layoutTiles) : computeColumnLayout(layoutTiles);
  }, [layoutTiles, isVertical]);

  const edges = useMemo(
    () => (positions && !isVertical ? computeRelationshipEdges(roles) : []),
    [positions, isVertical, roles],
  );

  const centerX = containerW / 2;
  const centerY = containerH / 2;
  const ready = containerW > 0 && positions !== null;

  useEffect(() => {
    if (ready && !hasAnimatedIn.current) {
      const id = setTimeout(() => { hasAnimatedIn.current = true; }, count * 40 + 600);
      return () => clearTimeout(id);
    }
  }, [ready, count]);

  return (
    <div ref={containerRef} className="relative h-full flex-1 overflow-hidden">
      {ready && positions && !isVertical && edges.length > 0 && (
        <svg className="pointer-events-none absolute inset-0 h-full w-full">
          {edges.map((edge) => (
            <motion.line
              key={`${keyOf(edge.from)}-${keyOf(edge.to)}`}
              x1={centerX + positions[edge.from].x}
              y1={centerY + positions[edge.from].y}
              x2={centerX + positions[edge.to].x}
              y2={centerY + positions[edge.to].y}
              className="stroke-foreground/15"
              strokeWidth={1}
              initial={{ pathLength: 0, opacity: 0 }}
              animate={{ pathLength: 1, opacity: 1 }}
              transition={{ duration: 0.5, ease: "easeOut" }}
            />
          ))}
        </svg>
      )}

      {/* Positioned with transforms, which don't affect the measured border box,
          so placement never feeds back into the measurement that drives it. A
          tile without a solved position waits invisibly at center to be measured. */}
      {containerW > 0 && (
        <AnimatePresence>
          {Array.from({ length: count }, (_, i) => {
            const pos = positions?.[i];
            return (
              <motion.div
                key={keyOf(i)}
                ref={measureRef(i)}
                className="absolute left-0 top-0"
                transformTemplate={(_, generated) => `${generated} translate(-50%, -50%)`}
                initial={hasAnimatedIn.current ? false : { opacity: 0, scale: 0.8, x: centerX, y: centerY }}
                animate={
                  pos
                    ? { opacity: 1, scale: 1, x: centerX + pos.x, y: centerY + pos.y }
                    : { opacity: 0, scale: 0.8, x: centerX, y: centerY }
                }
                exit={{ opacity: 0, scale: 0.8 }}
                transition={
                  hasAnimatedIn.current
                    ? { type: "spring", bounce: 0.1, duration: 0.4 }
                    : { type: "spring", bounce: 0.15, duration: 0.6, delay: i * 0.04 }
                }
              >
                {renderTile(i)}
              </motion.div>
            );
          })}
        </AnimatePresence>
      )}
    </div>
  );
}
