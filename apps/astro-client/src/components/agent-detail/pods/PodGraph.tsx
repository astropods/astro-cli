import { useEffect, useRef, useMemo, useCallback, type ReactNode } from "react";
import { motion, AnimatePresence } from "motion/react";
import { Plus, Minus, Maximize2 } from "lucide-react";
import { computeColumnLayout, type LayoutTile, type Position } from "./pod-layout";
import { computeRelationshipEdges } from "./pod-edges";
import { classify, type Role } from "./classify";
import { useTileMeasurements } from "./use-tile-measurements";
import { usePanZoom, type Bounds } from "./use-pan-zoom";
import { useContainerSize } from "@/hooks/use-container-size";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const VERTICAL_BREAKPOINT = 750;
const ZOOM_STEP = 1.25;
const CANVAS_SPRING = { type: "spring" as const, bounce: 0.15, duration: 0.5 };
// Soft top edge on the mobile scroll list, identical to the sibling agent-detail
// pages. The scroll region is inset below the top chrome, so tiles clip out of
// it entirely rather than showing through; this just softens the entry.
const SCROLL_FADE_MASK = "linear-gradient(to bottom, transparent, black 2rem)";

function CanvasControl({ label, icon: Icon, onClick }: {
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  onClick: () => void;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          aria-label={label}
          onClick={onClick}
          className="flex size-7 items-center justify-center text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
        >
          <Icon className="size-3.5" />
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

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
  /** Padding (px) that keeps the mobile scroll list clear of overlapping page
   * chrome — the top bar above and the deployment panel below. */
  insetTop?: number;
  insetBottom?: number;
}

/**
 * Layout-only component. Measures tiles continuously ({@link useTileMeasurements}),
 * places them in deterministic role columns ({@link computeColumnLayout}), and
 * draws relationship edges ({@link computeRelationshipEdges}) inside a
 * pan/zoom canvas ({@link usePanZoom}) so large graphs stay navigable. Collapses
 * to a single vertical stack below {@link VERTICAL_BREAKPOINT}.
 */
export function PodGraph({ count, renderTile, components, kinds, keys, effectiveWidth, insetTop = 0, insetBottom = 0 }: PodGraphProps) {
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
  const canvasEnabled = !isVertical;

  // Wait until every tile for the current count is measured; the length check
  // rejects a stale render between a count change and the next measure.
  const sizesValid = sizes.length === count && sizes.every(Boolean);

  const layoutTiles = useMemo<LayoutTile[] | null>(
    () => (sizesValid ? roles.map((role, i) => ({ role, size: sizes[i]! })) : null),
    [sizesValid, roles, sizes],
  );

  const positions = useMemo<Position[] | null>(
    () => (layoutTiles && !isVertical ? computeColumnLayout(layoutTiles) : null),
    [layoutTiles, isVertical],
  );

  const edges = useMemo(
    () => (positions && !isVertical ? computeRelationshipEdges(roles) : []),
    [positions, isVertical, roles],
  );

  const centerX = containerW / 2;
  const centerY = containerH / 2;
  const ready = containerW > 0 && positions !== null;

  // Mirrors `bounds` so usePanZoom's pan-clamp reads the current extent.
  const boundsRef = useRef<Bounds | null>(null);
  const { view, panning, autoFit, fit, resetView, zoomBy, panHandlers } = usePanZoom(containerRef, canvasEnabled, boundsRef);

  // Content bounding box in the identity (unscaled) frame — drives fit-to-view.
  const bounds = useMemo<Bounds | null>(() => {
    if (!positions || !sizesValid || containerW === 0) return null;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (let i = 0; i < count; i++) {
      const p = positions[i];
      const s = sizes[i];
      if (!p || !s) continue;
      const cx = centerX + p.x;
      const cy = centerY + p.y;
      minX = Math.min(minX, cx - s.width / 2);
      maxX = Math.max(maxX, cx + s.width / 2);
      minY = Math.min(minY, cy - s.height / 2);
      maxY = Math.max(maxY, cy + s.height / 2);
    }
    return Number.isFinite(minX) ? { minX, minY, maxX, maxY } : null;
  }, [positions, sizes, sizesValid, centerX, centerY, count, containerW]);
  boundsRef.current = bounds;

  // Frame the graph once on first load. fit caps at scale 1, so a graph that
  // already fits comfortably just centers at natural size, while a larger one
  // scales down to fit. Left alone afterward so runtime changes don't re-jump.
  const didAutoFit = useRef(false);
  useEffect(() => {
    if (canvasEnabled && bounds && containerW > 0 && containerH > 0 && !didAutoFit.current) {
      didAutoFit.current = true;
      autoFit(bounds, containerW, containerH);
    }
  }, [canvasEnabled, bounds, containerW, containerH, autoFit]);

  // Snap back to centered/fit whenever the container is resized, so a leftover
  // pan/zoom can't strand the graph off-screen at the new size. Skips the
  // initial sizing (handled by the first-load auto-fit above).
  const prevSize = useRef({ w: containerW, h: containerH });
  useEffect(() => {
    const prev = prevSize.current;
    const changed = prev.w !== containerW || prev.h !== containerH;
    prevSize.current = { w: containerW, h: containerH };
    if (canvasEnabled && bounds && prev.w > 0 && prev.h > 0 && changed) {
      resetView(bounds, containerW, containerH);
    }
  }, [canvasEnabled, bounds, containerW, containerH, resetView]);

  useEffect(() => {
    if (containerW > 0 && !hasAnimatedIn.current) {
      const id = setTimeout(() => { hasAnimatedIn.current = true; }, count * 40 + 600);
      return () => clearTimeout(id);
    }
  }, [containerW, count]);

  // Mobile / narrow: a natively-scrolling vertical list. The scroll region is
  // inset into the clear area between the top chrome and the bottom deployment
  // panel, so tiles clip out of view under them rather than showing through; a
  // top fade mask softens the entry, matching the sibling agent-detail pages.
  if (isVertical) {
    return (
      <div ref={containerRef} className="relative h-full flex-1 overflow-hidden">
        <div
          className="absolute inset-x-0 overflow-x-hidden overflow-y-auto"
          style={{ top: insetTop, bottom: insetBottom, maskImage: SCROLL_FADE_MASK, WebkitMaskImage: SCROLL_FADE_MASK }}
        >
          <div className="flex min-h-full flex-col">
            <div className="m-auto flex w-full flex-col items-center gap-3 p-4">
              <AnimatePresence>
                {Array.from({ length: count }, (_, i) => (
                  <motion.div
                    key={keyOf(i)}
                    ref={measureRef(i)}
                    initial={hasAnimatedIn.current ? false : { opacity: 0, scale: 0.9 }}
                    animate={{ opacity: 1, scale: 1 }}
                    exit={{ opacity: 0, scale: 0.9 }}
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
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      className={cn(
        "relative h-full flex-1 overflow-hidden",
        // Stop macOS/Chrome trackpad gestures (swipe-to-navigate, pinch page
        // zoom) from hijacking canvas pan/zoom.
        canvasEnabled && "touch-none overscroll-none",
        canvasEnabled && (panning ? "cursor-grabbing" : "cursor-grab"),
      )}
      {...(canvasEnabled ? panHandlers : {})}
    >
      {containerW > 0 && (
        <motion.div
          className="absolute inset-0 origin-top-left"
          initial={false}
          animate={canvasEnabled ? { x: view.x, y: view.y, scale: view.k } : { x: 0, y: 0, scale: 1 }}
          transition={canvasEnabled && view.animate ? CANVAS_SPRING : { duration: 0 }}
        >
          {ready && positions && !isVertical && edges.length > 0 && (
            <svg className="pointer-events-none absolute inset-0 h-full w-full overflow-visible">
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
          <AnimatePresence>
            {Array.from({ length: count }, (_, i) => {
              const pos = positions?.[i];
              return (
                <motion.div
                  key={keyOf(i)}
                  ref={measureRef(i)}
                  data-pod-tile
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
        </motion.div>
      )}

      {canvasEnabled && ready && bounds && (
        <TooltipProvider delayDuration={300}>
          <div
            className="absolute left-3 z-20 flex flex-col overflow-hidden rounded-lg border border-border/70 bg-card/80 shadow-sm backdrop-blur-sm transition-[bottom] duration-300 ease-out"
            // Lift above a full-width bottom panel (the narrow window where the
            // history panel has gone full-width but the graph is still a canvas);
            // otherwise rest at the default bottom-3 (12px).
            style={{ bottom: Math.max(12, insetBottom) }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            <CanvasControl label="Zoom in" icon={Plus} onClick={() => zoomBy(ZOOM_STEP)} />
            <div className="h-px w-full bg-border/70" />
            <CanvasControl label="Zoom out" icon={Minus} onClick={() => zoomBy(1 / ZOOM_STEP)} />
            <div className="h-px w-full bg-border/70" />
            <CanvasControl label="Fit to view" icon={Maximize2} onClick={() => fit(bounds, containerW, containerH)} />
          </div>
        </TooltipProvider>
      )}
    </div>
  );
}
