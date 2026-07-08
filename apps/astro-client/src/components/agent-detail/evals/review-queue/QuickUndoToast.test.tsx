import { render, screen, cleanup, fireEvent, act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuickUndoToast } from "./QuickUndoToast";

afterEach(cleanup);

describe("QuickUndoToast", () => {
  it("renders the label and Undo", () => {
    render(
      <QuickUndoToast
        label="Marked as neutral"
        isUndoing={false}
        onUndo={() => {}}
        onDismiss={() => {}}
      />,
    );
    expect(screen.getByText("Marked as neutral")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /undo/i })).toBeInTheDocument();
  });

  it("clicking Undo calls onUndo", () => {
    const onUndo = vi.fn();
    render(
      <QuickUndoToast
        label="Marked as neutral"
        isUndoing={false}
        onUndo={onUndo}
        onDismiss={() => {}}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /undo/i }));
    expect(onUndo).toHaveBeenCalledOnce();
  });

  it("auto-dismisses after the timeout elapses", () => {
    vi.useFakeTimers();
    try {
      const onDismiss = vi.fn();
      render(
        <QuickUndoToast
          label="Marked as neutral"
          isUndoing={false}
          onUndo={() => {}}
          onDismiss={onDismiss}
        />,
      );
      expect(onDismiss).not.toHaveBeenCalled();
      act(() => {
        vi.advanceTimersByTime(8000);
      });
      expect(onDismiss).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });
});
