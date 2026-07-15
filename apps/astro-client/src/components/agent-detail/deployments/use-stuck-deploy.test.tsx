import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useDeployStuckByAge } from "./use-stuck-deploy";

const BASE = new Date("2026-07-09T12:00:00.000Z").getTime();

afterEach(() => {
  vi.useRealTimers();
});

describe("useDeployStuckByAge", () => {
  it("is already stuck on mount when the deploy started longer ago than the threshold", () => {
    // The key correctness case: a user returning to (or reloading) an
    // already-stuck deploy sees it immediately, since age is measured from the
    // server start timestamp, not component mount.
    vi.useFakeTimers();
    vi.setSystemTime(BASE);
    const startedAt = new Date(BASE - 2000).toISOString();
    const { result } = renderHook(() => useDeployStuckByAge(startedAt, true, 1000));
    expect(result.current).toBe(true);
  });

  it("becomes stuck once the remaining time elapses", () => {
    vi.useFakeTimers();
    vi.setSystemTime(BASE);
    const startedAt = new Date(BASE).toISOString();
    const { result } = renderHook(() => useDeployStuckByAge(startedAt, true, 1000));
    expect(result.current).toBe(false);
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current).toBe(true);
  });

  it("is not stuck before the threshold elapses", () => {
    vi.useFakeTimers();
    vi.setSystemTime(BASE);
    const startedAt = new Date(BASE).toISOString();
    const { result } = renderHook(() => useDeployStuckByAge(startedAt, true, 1000));
    act(() => vi.advanceTimersByTime(900));
    expect(result.current).toBe(false);
  });

  it("never becomes stuck when not deploying", () => {
    vi.useFakeTimers();
    vi.setSystemTime(BASE);
    const startedAt = new Date(BASE - 5000).toISOString();
    const { result } = renderHook(() => useDeployStuckByAge(startedAt, false, 1000));
    act(() => vi.advanceTimersByTime(5000));
    expect(result.current).toBe(false);
  });

  it("is not stuck without a start timestamp", () => {
    vi.useFakeTimers();
    vi.setSystemTime(BASE);
    const { result } = renderHook(() => useDeployStuckByAge(undefined, true, 1000));
    act(() => vi.advanceTimersByTime(5000));
    expect(result.current).toBe(false);
  });

  it("resets to not-stuck when the deploy ends", () => {
    vi.useFakeTimers();
    vi.setSystemTime(BASE);
    const startedAt = new Date(BASE - 2000).toISOString();
    const { result, rerender } = renderHook(
      ({ dep }) => useDeployStuckByAge(startedAt, dep, 1000),
      { initialProps: { dep: true } },
    );
    expect(result.current).toBe(true);
    rerender({ dep: false });
    expect(result.current).toBe(false);
  });
});
