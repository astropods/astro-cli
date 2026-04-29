import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { setExperiment, useExperiments } from "./experiments";

function resetExperiments() {
  // Reset the module-level snapshot to defaults; localStorage.clear alone
  // doesn't suffice because the snapshot is captured at module init.
  setExperiment("theming", false);
  setExperiment("knowledgeStore", false);
  localStorage.clear();
}

afterEach(() => {
  cleanup();
  resetExperiments();
});

beforeEach(() => {
  resetExperiments();
});

function ExperimentsProbe({ id }: { id: string }) {
  const { experiments, setExperiment } = useExperiments();
  return (
    <div>
      <span data-testid={`${id}-theming`}>{String(experiments.theming)}</span>
      <button
        type="button"
        onClick={() => setExperiment("theming", !experiments.theming)}
        data-testid={`${id}-toggle`}
      >
        toggle
      </button>
    </div>
  );
}

describe("useExperiments", () => {
  it("toggling in one consumer updates a sibling consumer in the same tree (no reload)", () => {
    render(
      <>
        <ExperimentsProbe id="a" />
        <ExperimentsProbe id="b" />
      </>,
    );

    expect(screen.getByTestId("a-theming")).toHaveTextContent("false");
    expect(screen.getByTestId("b-theming")).toHaveTextContent("false");

    fireEvent.click(screen.getByTestId("a-toggle"));

    // Both consumers must reflect the new value without any remount or reload.
    expect(screen.getByTestId("a-theming")).toHaveTextContent("true");
    expect(screen.getByTestId("b-theming")).toHaveTextContent("true");
  });

  it("persists across tree unmount/remount via localStorage", () => {
    const first = render(<ExperimentsProbe id="a" />);
    fireEvent.click(screen.getByTestId("a-toggle"));
    expect(screen.getByTestId("a-theming")).toHaveTextContent("true");
    first.unmount();

    render(<ExperimentsProbe id="b" />);
    expect(screen.getByTestId("b-theming")).toHaveTextContent("true");
  });

  it("setExperiment writes through to localStorage", () => {
    render(<ExperimentsProbe id="a" />);
    fireEvent.click(screen.getByTestId("a-toggle"));

    const stored = JSON.parse(localStorage.getItem("astro:experiments") ?? "{}");
    expect(stored.theming).toBe(true);
  });
});
