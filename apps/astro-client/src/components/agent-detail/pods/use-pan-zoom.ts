import { useCallback, useEffect, useRef, useState, type PointerEvent, type RefObject } from "react";

/** Pan/zoom transform: screen = translate(x, y) then scale(k), origin top-left. */
export interface View {
  x: number;
  y: number;
  k: number;
}

/** View plus whether the last change should spring-animate (discrete actions) or apply instantly (drag/wheel). */
export type ViewState = View & { animate: boolean };

/** Bounding box of the content in the identity (unscaled) coordinate frame. */
export interface Bounds {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
}

export const MIN_ZOOM = 0.2;
export const MAX_ZOOM = 2.5;
const FIT_PADDING = 48;
const WHEEL_ZOOM_SENSITIVITY = 0.01;
const PAN_TILE_SELECTOR = "[data-pod-tile]";

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}

// --- Pure view transforms (exported for testing) ---

/** Zoom `view` by `factor` toward screen point (px, py); scale is clamped to
 * [MIN_ZOOM, MAX_ZOOM] and the world point under (px, py) stays fixed. */
export function zoomToward(view: View, px: number, py: number, factor: number): View {
  const k = clamp(view.k * factor, MIN_ZOOM, MAX_ZOOM);
  const s = k / view.k;
  return { k, x: px - (px - view.x) * s, y: py - (py - view.y) * s };
}

/** Shift `view` so the content bounds' center stays within the viewport — the
 * content can sit at most halfway off any edge, never fully out of sight. */
export function clampPanView(view: View, bounds: Bounds, width: number, height: number): View {
  const centerX = view.x + view.k * (bounds.minX + bounds.maxX) / 2;
  const centerY = view.y + view.k * (bounds.minY + bounds.maxY) / 2;
  return {
    ...view,
    x: view.x + (clamp(centerX, 0, width) - centerX),
    y: view.y + (clamp(centerY, 0, height) - centerY),
  };
}

/** Center `bounds` in the viewport and scale to fit (capped at 1), or null when
 * the bounds or viewport are degenerate. */
export function fitToViewport(bounds: Bounds, width: number, height: number): View | null {
  const bw = bounds.maxX - bounds.minX;
  const bh = bounds.maxY - bounds.minY;
  if (bw <= 0 || bh <= 0 || width === 0 || height === 0) return null;
  const k = clamp(Math.min((width - 2 * FIT_PADDING) / bw, (height - 2 * FIT_PADDING) / bh, 1), MIN_ZOOM, MAX_ZOOM);
  const cx = (bounds.minX + bounds.maxX) / 2;
  const cy = (bounds.minY + bounds.maxY) / 2;
  return { k, x: width / 2 - k * cx, y: height / 2 - k * cy };
}

/**
 * Turns a container into a pannable/zoomable canvas whose content is placed in
 * a top-left-anchored `translate/scale` world layer.
 *
 * - trackpad/mouse wheel pans; ctrl/⌘ + wheel (and trackpad pinch) zooms toward
 *   the cursor; dragging empty canvas pans; buttons zoom and fit.
 * - `autoFit` frames the content until the user first interacts; `fit` (the
 *   button) recenters and resumes auto-fitting.
 * - `view.animate` marks button-driven changes so the caller can spring to
 *   them, while drag/wheel updates apply instantly to track the input 1:1.
 */
export function usePanZoom(
  containerRef: RefObject<HTMLElement | null>,
  enabled: boolean,
  boundsRef: RefObject<Bounds | null>,
) {
  const [view, setView] = useState<ViewState>({ x: 0, y: 0, k: 1, animate: false });
  const [panning, setPanning] = useState(false);
  const userAdjusted = useRef(false);
  const drag = useRef<{ startX: number; startY: number; originX: number; originY: number } | null>(null);

  // Apply the pan clamp against the live viewport + bounds, preserving the flag.
  const clampPan = useCallback((v: ViewState): ViewState => {
    const el = containerRef.current;
    const b = boundsRef.current;
    if (!el || !b) return v;
    const { width, height } = el.getBoundingClientRect();
    return { ...clampPanView(v, b, width, height), animate: v.animate };
  }, [containerRef, boundsRef]);

  const zoomAt = useCallback((px: number, py: number, factor: number, animate: boolean) => {
    setView((v) => clampPan({ ...zoomToward(v, px, py, factor), animate }));
  }, [clampPan]);

  // Native listener so we can preventDefault — React's onWheel is passive.
  useEffect(() => {
    const el = containerRef.current;
    if (!el || !enabled) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      userAdjusted.current = true;
      const rect = el.getBoundingClientRect();
      if (e.ctrlKey || e.metaKey) {
        zoomAt(e.clientX - rect.left, e.clientY - rect.top, Math.exp(-e.deltaY * WHEEL_ZOOM_SENSITIVITY), false);
      } else {
        setView((v) => clampPan({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY, animate: false }));
      }
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [containerRef, enabled, zoomAt, clampPan]);

  const onPointerDown = useCallback((e: PointerEvent<HTMLElement>) => {
    // Pan only when grabbing empty canvas — a tile handles its own click.
    if (!enabled || (e.target as HTMLElement).closest(PAN_TILE_SELECTOR)) return;
    drag.current = { startX: e.clientX, startY: e.clientY, originX: view.x, originY: view.y };
    userAdjusted.current = true;
    setPanning(true);
    e.currentTarget.setPointerCapture(e.pointerId);
  }, [enabled, view.x, view.y]);

  const onPointerMove = useCallback((e: PointerEvent<HTMLElement>) => {
    const d = drag.current;
    if (!d) return;
    setView((v) => clampPan({ ...v, x: d.originX + (e.clientX - d.startX), y: d.originY + (e.clientY - d.startY), animate: false }));
  }, [clampPan]);

  const endPan = useCallback(() => {
    drag.current = null;
    setPanning(false);
  }, []);

  // force: fit even after the user has panned/zoomed (and re-arm auto-fit).
  // animate: spring to the framing vs. snap instantly.
  const fitView = useCallback(
    (bounds: Bounds, width: number, height: number, opts: { force: boolean; animate: boolean }) => {
      if (!opts.force && userAdjusted.current) return;
      const framed = fitToViewport(bounds, width, height);
      if (!framed) return;
      setView({ ...framed, animate: opts.animate });
      if (opts.force) userAdjusted.current = false;
    },
    [],
  );

  // autoFit: passive first framing, skipped once the user takes control.
  const autoFit = useCallback((b: Bounds, w: number, h: number) => fitView(b, w, h, { force: false, animate: false }), [fitView]);
  // fit: the control button — always re-frames, with a spring.
  const fit = useCallback((b: Bounds, w: number, h: number) => fitView(b, w, h, { force: true, animate: true }), [fitView]);
  // resetView: snap back to centered/fit on a viewport resize so a pan/zoom
  // can't leave the graph stranded off-screen.
  const resetView = useCallback((b: Bounds, w: number, h: number) => fitView(b, w, h, { force: true, animate: false }), [fitView]);

  const zoomBy = useCallback((factor: number) => {
    const el = containerRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    userAdjusted.current = true;
    zoomAt(rect.width / 2, rect.height / 2, factor, true);
  }, [containerRef, zoomAt]);

  return {
    view,
    panning,
    autoFit,
    fit,
    resetView,
    zoomBy,
    panHandlers: { onPointerDown, onPointerMove, onPointerUp: endPan, onPointerCancel: endPan },
  };
}
