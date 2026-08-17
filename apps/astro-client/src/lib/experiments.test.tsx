import { describe, it, expect, afterEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { renderHook, act } from "@testing-library/react";
import { useExperiments, hasExperiments, setExperiment } from "./experiments";

// Cast to exercise the store plumbing (subscribe/notify/persist) with an
// arbitrary key without modifying production types.
const setTestExperiment = setExperiment as unknown as (key: string, value: boolean) => void;

function resetStore() {
  setTestExperiment("__test__", false);
  localStorage.clear();
}

afterEach(() => {
  cleanup();
  resetStore();
});

describe("useExperiments", () => {
  it("returns default experiment values", () => {
    const { result } = renderHook(() => useExperiments());
    // Every experiment must default to off, so a new one can't ship enabled by
    // accident. Asserting the whole object rather than individual keys is what
    // makes that a compile-and-test failure when one is added.
    expect(result.current.experiments).toEqual({ evals: false });
  });

  it("hasExperiments is true when experiments are defined", () => {
    expect(hasExperiments).toBe(true);
  });
});

describe("store reactive infrastructure", () => {
  it("setExperiment notifies all active consumers in the same tree without remount", () => {
    function Probe({ id }: { id: string }) {
      const { experiments } = useExperiments();
      return <span data-testid={id}>{String((experiments as unknown as Record<string, unknown>).__test__ ?? false)}</span>;
    }

    render(<><Probe id="a" /><Probe id="b" /></>);

    expect(screen.getByTestId("a")).toHaveTextContent("false");
    expect(screen.getByTestId("b")).toHaveTextContent("false");

    act(() => setTestExperiment("__test__", true));

    expect(screen.getByTestId("a")).toHaveTextContent("true");
    expect(screen.getByTestId("b")).toHaveTextContent("true");
  });

  it("persists state across unmount and remount via localStorage", () => {
    function Probe() {
      const { experiments } = useExperiments();
      return <span data-testid="val">{String((experiments as unknown as Record<string, unknown>).__test__ ?? false)}</span>;
    }

    const first = render(<Probe />);
    act(() => setTestExperiment("__test__", true));
    expect(screen.getByTestId("val")).toHaveTextContent("true");
    first.unmount();

    render(<Probe />);
    expect(screen.getByTestId("val")).toHaveTextContent("true");
  });

  it("setExperiment writes through to localStorage", () => {
    act(() => setTestExperiment("__test__", true));
    const stored = JSON.parse(localStorage.getItem("astro:experiments") ?? "{}");
    expect(stored.__test__).toBe(true);
  });

  it("setExperiment is a no-op when the value is unchanged", () => {
    act(() => setTestExperiment("__test__", false));
    const { result } = renderHook(() => useExperiments());
    const snapshotBefore = result.current.experiments;
    act(() => setTestExperiment("__test__", false));
    expect(result.current.experiments).toBe(snapshotBefore);
  });
});
