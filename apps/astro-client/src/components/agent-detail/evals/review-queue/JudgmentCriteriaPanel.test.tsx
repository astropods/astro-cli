import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { JudgmentCriteriaPanel } from "./JudgmentCriteriaPanel";

afterEach(cleanup);

function renderPanel(
  overrides: Partial<Parameters<typeof JudgmentCriteriaPanel>[0]> = {},
) {
  const onSave = vi.fn();
  const onUndo = vi.fn();
  render(
    <JudgmentCriteriaPanel
      isUndoing={false}
      isSaving={false}
      isError={false}
      onUndo={onUndo}
      onSave={onSave}
      {...overrides}
    />,
  );
  return { onSave, onUndo };
}

describe("JudgmentCriteriaPanel", () => {
  it("renders the neutral title, Undo, Optional badge, and good-side labels", () => {
    renderPanel();
    expect(screen.getByText("Evaluate trace")).toHaveClass("text-heading-4");
    expect(screen.queryByText("Marked as good")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /undo/i })).toBeInTheDocument();
    expect(screen.getByText("Optional")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /correct info/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /followed instruction/i }),
    ).toBeInTheDocument();
  });

  it("clicking Undo calls onUndo", () => {
    const { onUndo } = renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /undo/i }));
    expect(onUndo).toHaveBeenCalledOnce();
  });

  it("shows an error message when the save failed", () => {
    renderPanel({ isError: true });
    expect(screen.getByText(/could not save criteria/i)).toBeInTheDocument();
  });

  it("shows no error message by default", () => {
    renderPanel();
    expect(screen.queryByText(/could not save criteria/i)).not.toBeInTheDocument();
  });

  it("toggling a chip flips its data-active state", () => {
    renderPanel();
    const chip = screen.getByRole("button", { name: /correct info/i });
    expect(chip).not.toHaveAttribute("data-active");
    fireEvent.click(chip);
    expect(chip).toHaveAttribute("data-active");
    fireEvent.click(chip);
    expect(chip).not.toHaveAttribute("data-active");
  });

  it("focuses the first criterion when opened", async () => {
    renderPanel();
    const firstCriterion = screen.getByRole("button", {
      name: /correct info/i,
    });

    await waitFor(() => expect(firstCriterion).toHaveFocus());
  });

  it("Save emits selected criteria as positive and omits unselected criteria", () => {
    const { onSave } = renderPanel();
    // Select out of display order to prove the output is ordered.
    fireEvent.click(screen.getByRole("button", { name: /followed instruction/i }));
    fireEvent.click(screen.getByRole("button", { name: /correct info/i }));
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).toHaveBeenCalledWith([
      { dimension_key: "accuracy", value: 1 },
      { dimension_key: "instruction_following", value: 1 },
    ]);
  });

  it("starts with supplied prediction reasons selected", () => {
    const { onSave } = renderPanel({
      initialKeys: ["accuracy", "tone"],
    });

    expect(
      screen.getByRole("button", { name: /correct info/i }),
    ).toHaveAttribute("data-active");
    expect(
      screen.getByRole("button", { name: /appropriate tone/i }),
    ).toHaveAttribute("data-active");

    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).toHaveBeenCalledWith([
      { dimension_key: "accuracy", value: 1 },
      { dimension_key: "tone", value: 1 },
    ]);
  });

  it("Save with no criteria selected omits every criterion", () => {
    const { onSave } = renderPanel();
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).toHaveBeenCalledWith([]);
  });
});
