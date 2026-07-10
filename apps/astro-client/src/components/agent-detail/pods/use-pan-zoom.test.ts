import { describe, it, expect } from "vitest";
import { zoomToward, clampPanView, fitToViewport, MIN_ZOOM, MAX_ZOOM, type Bounds, type View } from "./use-pan-zoom";

// Screen position of a world point under a view: screen = view.x + view.k * world.
const project = (view: View, wx: number, wy: number) => ({ x: view.x + view.k * wx, y: view.y + view.k * wy });
const centerOf = (b: Bounds) => ({ x: (b.minX + b.maxX) / 2, y: (b.minY + b.maxY) / 2 });

describe("fitToViewport", () => {
  const W = 1000, H = 600;

  it("centers the bounds in the viewport", () => {
    const bounds: Bounds = { minX: 200, minY: 100, maxX: 800, maxY: 500 };
    const view = fitToViewport(bounds, W, H)!;
    const c = centerOf(bounds);
    const screen = project(view, c.x, c.y);
    expect(screen.x).toBeCloseTo(W / 2, 5);
    expect(screen.y).toBeCloseTo(H / 2, 5);
  });

  it("caps scale at 1 for a graph that already fits", () => {
    const bounds: Bounds = { minX: 400, minY: 250, maxX: 600, maxY: 350 }; // 200x100, fits easily
    expect(fitToViewport(bounds, W, H)!.k).toBe(1);
  });

  it("scales a large graph down within [MIN_ZOOM, 1]", () => {
    const bounds: Bounds = { minX: 0, minY: 0, maxX: 4000, maxY: 3000 };
    const { k } = fitToViewport(bounds, W, H)!;
    expect(k).toBeLessThan(1);
    expect(k).toBeGreaterThanOrEqual(MIN_ZOOM);
  });

  it("returns null for a degenerate viewport or bounds", () => {
    expect(fitToViewport({ minX: 0, minY: 0, maxX: 100, maxY: 100 }, 0, 0)).toBeNull();
    expect(fitToViewport({ minX: 0, minY: 0, maxX: 0, maxY: 0 }, W, H)).toBeNull();
  });
});

describe("zoomToward", () => {
  it("keeps the world point under the anchor pixel fixed", () => {
    const view: View = { x: 30, y: -20, k: 1.2 };
    const [px, py] = [420, 260];
    const before = (px - view.x) / view.k; // world x under the anchor
    const after = zoomToward(view, px, py, 1.5);
    expect((px - after.x) / after.k).toBeCloseTo(before, 5);
    // and the y axis
    expect((py - after.y) / after.k).toBeCloseTo((py - view.y) / view.k, 5);
  });

  it("clamps scale to MIN_ZOOM and MAX_ZOOM", () => {
    const view: View = { x: 0, y: 0, k: 1 };
    expect(zoomToward(view, 0, 0, 0.001).k).toBe(MIN_ZOOM);
    expect(zoomToward(view, 0, 0, 1000).k).toBe(MAX_ZOOM);
  });
});

describe("clampPanView", () => {
  const bounds: Bounds = { minX: 0, minY: 0, maxX: 400, maxY: 300 };
  const W = 1000, H = 600;
  const screenCenter = (v: View) => {
    const c = centerOf(bounds);
    return project(v, c.x, c.y);
  };

  it("leaves a view whose content center is already inside the viewport unchanged", () => {
    const view: View = { x: 100, y: 80, k: 1 };
    expect(clampPanView(view, bounds, W, H)).toEqual(view);
  });

  it("pulls content back when its center is pushed past an edge", () => {
    // Panned far right/down so the content center sits beyond the viewport.
    const pushed: View = { x: 5000, y: 4000, k: 1 };
    const clamped = clampPanView(pushed, bounds, W, H);
    const c = screenCenter(clamped);
    expect(c.x).toBeCloseTo(W, 5); // center pinned to the right edge
    expect(c.y).toBeCloseTo(H, 5); // center pinned to the bottom edge
  });

  it("pins the center to the near edge when panned past the top-left", () => {
    const pushed: View = { x: -5000, y: -4000, k: 1 };
    const c = screenCenter(clampPanView(pushed, bounds, W, H));
    expect(c.x).toBeCloseTo(0, 5);
    expect(c.y).toBeCloseTo(0, 5);
  });
});
