import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EvaluationSetEvaluator } from "@/lib/api";
import { ReviewQueueEvaluationSection } from "./ReviewQueueEvaluationSection";

afterEach(cleanup);

const EVALUATORS: EvaluationSetEvaluator[] = [
  {
    key: "exposed_pii",
    label: "Exposed PII",
    type: "llm",
    output: { type: "boolean" },
  },
];

function renderSection(
  props: Partial<
    React.ComponentProps<typeof ReviewQueueEvaluationSection>
  > = {},
) {
  const onAdd = vi.fn();
  const onRemove = vi.fn();
  const onOpenChange = vi.fn();
  const view = render(
    <ReviewQueueEvaluationSection
      evaluators={EVALUATORS}
      results={[]}
      scored={false}
      open
      onOpenChange={onOpenChange}
      isSaving={false}
      isRemoving={false}
      onAdd={onAdd}
      onRemove={onRemove}
      {...props}
    />,
  );
  return { ...view, onAdd, onOpenChange, onRemove };
}

describe("ReviewQueueEvaluationSection", () => {
  it("commits the values on save", async () => {
    const user = userEvent.setup();
    const { onAdd } = renderSection();

    await user.click(screen.getByRole("combobox", { name: "Exposed PII" }));
    await user.click(screen.getByRole("option", { name: "True" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(onAdd).toHaveBeenCalledWith(
      [{ key: "exposed_pii", value: true }],
      expect.any(HTMLElement),
    );
  });

  it("admits a trace the reviewer left unlabelled", async () => {
    const user = userEvent.setup();
    const { onAdd } = renderSection();

    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(onAdd).toHaveBeenCalledWith([], expect.any(HTMLElement));
  });

  it("reports a failed save and keeps the action available", () => {
    renderSection({ addError: "Could not add to the dataset. Try again." });

    expect(
      screen.getByText("Could not add to the dataset. Try again."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("blocks a second save while one is in flight", () => {
    renderSection({ isSaving: true });

    expect(screen.getByRole("button", { name: "Saving..." })).toBeDisabled();
  });

  it("blocks saving until the first result arrives", () => {
    renderSection({ loading: true });

    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("explains a run that came back with nothing", () => {
    renderSection({ attempted: true });

    expect(
      screen.getByText(/The evaluator couldn’t score this trace/),
    ).toBeInTheDocument();
  });

  it("leaves a trace nobody evaluated unexplained", () => {
    renderSection();

    expect(
      screen.queryByText(/The evaluator couldn’t score this trace/),
    ).not.toBeInTheDocument();
  });

  it("opens from the add action while collapsed", async () => {
    const user = userEvent.setup();
    const { onOpenChange } = renderSection({ open: false });

    await user.click(screen.getByRole("button", { name: "Add to dataset" }));

    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("toggles from anywhere in the header", () => {
    const { onOpenChange } = renderSection({ open: false });

    screen.getByText("Evaluate trace").click();

    expect(onOpenChange).toHaveBeenCalledWith(true);
  });

  it("titles a scored trace by its results", () => {
    renderSection({ scored: true });

    expect(screen.getByText("Evaluation results")).toBeInTheDocument();
    expect(screen.queryByText("No results")).not.toBeInTheDocument();
  });

  it("flags a run that came back empty", () => {
    renderSection({ attempted: true });

    expect(screen.getByText("Evaluation results")).toBeInTheDocument();
    expect(screen.getByText("No results")).toBeInTheDocument();
  });

  it("removes the trace without adding it", async () => {
    const user = userEvent.setup();
    const { onAdd, onRemove } = renderSection();

    await user.click(screen.getByRole("button", { name: "Remove" }));

    expect(onRemove).toHaveBeenCalledTimes(1);
    expect(onAdd).not.toHaveBeenCalled();
  });

  it("disables both actions while removing", () => {
    renderSection({ isRemoving: true });

    expect(screen.getByRole("button", { name: "Removing..." })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Add to dataset" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });
});
