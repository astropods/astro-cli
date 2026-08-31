import { describe, expect, test, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useInViewport } from "./use-in-viewport";

type Cb = (entries: { isIntersecting: boolean }[]) => void;

let callbacks: Cb[] = [];
const observe = vi.fn();
const disconnect = vi.fn();

beforeEach(() => {
  callbacks = [];
  observedRoots = [];
  observe.mockClear();
  disconnect.mockClear();
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(cb: Cb, opts?: { root?: Element | null }) {
        callbacks.push(cb);
        observedRoots.push(opts?.root ?? null);
      }
      observe = observe;
      disconnect = disconnect;
      unobserve = vi.fn();
      takeRecords = vi.fn();
      root = null;
      rootMargin = "";
      thresholds = [];
    },
  );
});

afterEach(() => vi.unstubAllGlobals());

let observedRoots: (Element | null)[] = [];

function mounted(scrollable = false) {
  return renderHook(() => {
    const r = useInViewport<HTMLDivElement>();
    if (!r.ref.current) {
      const el = document.createElement("div");
      if (scrollable) {
        const container = document.createElement("div");
        container.style.overflowY = "auto";
        container.appendChild(el);
        document.body.appendChild(container);
      }
      r.ref.current = el;
    }
    return r;
  });
}

describe("useInViewport", () => {
  test("starts out of view", () => {
    const { result } = mounted();
    expect(result.current.inViewport).toBe(false);
  });

  test("reports in view once the element intersects", () => {
    const { result } = mounted();
    act(() => callbacks[0]([{ isIntersecting: true }]));
    expect(result.current.inViewport).toBe(true);
  });

  test("reports out of view again once the element leaves", () => {
    const { result } = mounted();
    act(() => callbacks[0]([{ isIntersecting: true }]));
    act(() => callbacks[0]([{ isIntersecting: false }]));
    expect(result.current.inViewport).toBe(false);
  });

  test("disconnects the observer on unmount", () => {
    const { unmount } = mounted();
    unmount();
    expect(disconnect).toHaveBeenCalled();
  });

  test("degrades to eager when IntersectionObserver is missing", () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    const { result } = renderHook(() => useInViewport<HTMLDivElement>());
    expect(result.current.inViewport).toBe(true);
  });

  // The thread scrolls inside its own container, so observing against the
  // viewport reports a clipped element as off screen and the preview never
  // loads.
  test("observes against the nearest scrolling ancestor", () => {
    mounted(true);

    expect(observedRoots[0]).not.toBeNull();
    expect((observedRoots[0] as HTMLElement).style.overflowY).toBe("auto");
  });

  test("falls back to the viewport with no scrolling ancestor", () => {
    mounted(false);

    expect(observedRoots[0]).toBeNull();
  });
});
