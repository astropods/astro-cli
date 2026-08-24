import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TraceEvaluatorResult } from "@/lib/api";
import { ReviewQueueEvaluationControls } from "./ReviewQueueEvaluationControls";
import { ReviewQueueEvaluationResults } from "./ReviewQueueEvaluationResults";

function result(
  overrides: Partial<TraceEvaluatorResult> = {},
): TraceEvaluatorResult {
  return {
    key: "exposed_pii",
    label: "Exposed PII",
    description: "Flags personal data in the output.",
    status: "completed",
    value: false,
    confidence: 0.82,
    explanation: "No personal data appeared in the answer.",
    error: null,
    ...overrides,
  };
}

describe("ReviewQueueEvaluationControls", () => {
  it("toggles from anywhere in the header", async () => {
    const onOpenChange = vi.fn();
    render(
      <ReviewQueueEvaluationControls
        resultsOpen={false}
        onResultsOpenChange={onOpenChange}
      />,
    );

    screen.getByText("Evaluation results").click();

    expect(onOpenChange).toHaveBeenCalledWith(true);
  });

  it("exposes the chevron as the labelled disclosure control", () => {
    const onOpenChange = vi.fn();
    const { rerender } = render(
      <ReviewQueueEvaluationControls
        resultsOpen={false}
        onResultsOpenChange={onOpenChange}
      />,
    );

    const expand = screen.getByRole("button", {
      name: /expand evaluation results/i,
    });
    expect(expand).toHaveAttribute("aria-expanded", "false");
    expand.click();
    expect(onOpenChange).toHaveBeenCalledWith(true);

    rerender(
      <ReviewQueueEvaluationControls
        resultsOpen
        onResultsOpenChange={onOpenChange}
      />,
    );
    expect(
      screen.getByRole("button", { name: /collapse evaluation results/i }),
    ).toHaveAttribute("aria-expanded", "true");
  });

  it("leaves nested actions to their own handlers", () => {
    const onOpenChange = vi.fn();
    render(
      <ReviewQueueEvaluationControls
        resultsOpen
        onResultsOpenChange={onOpenChange}
        actions={<button type="button">Remove</button>}
      />,
    );

    screen.getByRole("button", { name: "Remove" }).click();

    expect(onOpenChange).not.toHaveBeenCalled();
  });
});

describe("ReviewQueueEvaluationResults", () => {
  it("shows the evaluator, its reasoning, result, and confidence", () => {
    render(<ReviewQueueEvaluationResults evaluators={[result()]} />);

    expect(screen.getByText("Exposed PII")).toBeInTheDocument();
    expect(
      screen.getByText("No personal data appeared in the answer."),
    ).toBeInTheDocument();
    expect(screen.getByText("False")).toBeInTheDocument();
    expect(screen.getByText("82%")).toBeInTheDocument();
  });

  it("offers the definition beside the evaluator name", () => {
    render(<ReviewQueueEvaluationResults evaluators={[result()]} />);

    expect(
      screen.getByLabelText("About Exposed PII"),
    ).toBeInTheDocument();
  });

  it("omits the definition when the run used an unresolvable set", () => {
    render(
      <ReviewQueueEvaluationResults
        evaluators={[
          result({ key: "retired_check", label: undefined, description: undefined }),
        ]}
      />,
    );

    expect(screen.getByText("retired_check")).toBeInTheDocument();
    expect(screen.queryByLabelText(/^About /)).not.toBeInTheDocument();
  });

  it("titles an enum value", () => {
    render(
      <ReviewQueueEvaluationResults
        evaluators={[result({ key: "claim_grounding", value: "no_claims" })]}
      />,
    );

    expect(screen.getByText("No claims")).toBeInTheDocument();
  });

  it("shows an error result for a failed evaluator, with no confidence", () => {
    render(
      <ReviewQueueEvaluationResults
        evaluators={[
          result({
            status: "failed",
            value: null,
            explanation: "",
            error: "Bad output.",
          }),
        ]}
      />,
    );

    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(
      screen.getByText("Evaluation could not be generated."),
    ).toBeInTheDocument();
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("explains when nothing has been recorded", () => {
    render(<ReviewQueueEvaluationResults evaluators={[]} />);

    expect(
      screen.getByText(/couldn’t return a result for this trace/i),
    ).toBeInTheDocument();
  });
});
