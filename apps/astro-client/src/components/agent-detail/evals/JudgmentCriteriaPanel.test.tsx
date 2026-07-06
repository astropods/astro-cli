import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { JudgmentCriteriaPanel } from "./JudgmentCriteriaPanel";

afterEach(cleanup);

function renderPanel(
  verdict: "good" | "bad",
  overrides: Partial<Parameters<typeof JudgmentCriteriaPanel>[0]> = {},
) {
  const onDone = vi.fn();
  const onUndo = vi.fn();
  render(
    <JudgmentCriteriaPanel
      verdict={verdict}
      title={`Marked as ${verdict}`}
      isUndoing={false}
      isSaving={false}
      isError={false}
      onUndo={onUndo}
      onDone={onDone}
      {...overrides}
    />,
  );
  return { onDone, onUndo };
}

describe("JudgmentCriteriaPanel", () => {
  it("renders the title, Undo, good heading, Optional badge, and good-side labels", () => {
    renderPanel("good");
    expect(screen.getByText("Marked as good")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /undo/i })).toBeInTheDocument();
    expect(screen.getByText("Why is it good?")).toBeInTheDocument();
    expect(screen.getByText("Optional")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /correct info/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /followed instruction/i }),
    ).toBeInTheDocument();
  });

  it("renders bad-side labels for a bad verdict", () => {
    renderPanel("bad");
    expect(screen.getByText("Why is it bad?")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /hallucination/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /ignored instruction/i }),
    ).toBeInTheDocument();
  });

  it("clicking Undo calls onUndo", () => {
    const { onUndo } = renderPanel("good");
    fireEvent.click(screen.getByRole("button", { name: /undo/i }));
    expect(onUndo).toHaveBeenCalledOnce();
  });

  it("shows an error message when the save failed", () => {
    renderPanel("good", { isError: true });
    expect(screen.getByText(/could not save criteria/i)).toBeInTheDocument();
  });

  it("shows no error message by default", () => {
    renderPanel("good");
    expect(screen.queryByText(/could not save criteria/i)).not.toBeInTheDocument();
  });

  it("toggling a chip flips its data-active state", () => {
    renderPanel("good");
    const chip = screen.getByRole("button", { name: /correct info/i });
    expect(chip).not.toHaveAttribute("data-active");
    fireEvent.click(chip);
    expect(chip).toHaveAttribute("data-active");
    fireEvent.click(chip);
    expect(chip).not.toHaveAttribute("data-active");
  });

  it("Done emits selected criteria with value 1 for good, in display order", () => {
    const { onDone } = renderPanel("good");
    // Select out of display order to prove the output is ordered.
    fireEvent.click(screen.getByRole("button", { name: /followed instruction/i }));
    fireEvent.click(screen.getByRole("button", { name: /correct info/i }));
    fireEvent.click(screen.getByRole("button", { name: /^done$/i }));
    expect(onDone).toHaveBeenCalledWith([
      { dimension_key: "accuracy", value: 1 },
      { dimension_key: "instruction_following", value: 1 },
    ]);
  });

  it("Done emits value -1 for a bad verdict", () => {
    const { onDone } = renderPanel("bad");
    fireEvent.click(screen.getByRole("button", { name: /hallucination/i }));
    fireEvent.click(screen.getByRole("button", { name: /^done$/i }));
    expect(onDone).toHaveBeenCalledWith([{ dimension_key: "accuracy", value: -1 }]);
  });

  it("Done with no selection emits an empty array", () => {
    const { onDone } = renderPanel("good");
    fireEvent.click(screen.getByRole("button", { name: /^done$/i }));
    expect(onDone).toHaveBeenCalledWith([]);
  });
});
