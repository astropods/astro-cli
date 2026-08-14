import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ReviewQueuePrediction } from "@/lib/api";
import { ReviewQueuePredictionExplanation } from "./ReviewQueuePredictionExplanation";
import { ReviewQueuePredictionControls } from "./ReviewQueuePredictionControls";

const prediction: ReviewQueuePrediction = {
  verdict_score: -0.8,
  confidence: 79,
  explanation: "The response did not address the request.",
  judge_version: "1",
  criteria: [
    { dimension_key: "accuracy", dimension_value: -0.8 },
    { dimension_key: "completeness", dimension_value: -0.4 },
    { dimension_key: "instruction_following", dimension_value: 0 },
    { dimension_key: "scope_clarity", dimension_value: 0.6 },
    { dimension_key: "tone", dimension_value: 0.3 },
  ],
};

function PredictionExplanationHarness() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <ReviewQueuePredictionControls
        prediction={prediction}
        explanationOpen={open}
        onExplanationOpenChange={setOpen}
      />
      {open && <ReviewQueuePredictionExplanation prediction={prediction} />}
    </>
  );
}

describe("ReviewQueuePredictionControls", () => {
  it("expands and collapses the judge explanation", async () => {
    const user = userEvent.setup();
    render(<PredictionExplanationHarness />);

    expect(screen.getByText("Bad")).toBeInTheDocument();
    expect(screen.getByText("Bad").parentElement).toHaveStyle({
      color: "var(--destructive)",
    });
    expect(screen.queryByText("79% confident")).not.toBeInTheDocument();
    expect(
      screen.queryByText("The response did not address the request."),
    ).not.toBeInTheDocument();

    const explanationButton = screen.getByRole("button", {
      name: "See explanation",
    });
    expect(explanationButton).toHaveClass("whitespace-nowrap", "shrink-0");
    expect(explanationButton.parentElement).toHaveClass("flex-nowrap");

    await user.click(
      screen.getByRole("button", { name: "See explanation" }),
    );

    expect(
      screen.getByRole("button", { name: "Hide explanation" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Judge's verdict")).toBeInTheDocument();
    expect(
      screen.getByText("The response did not address the request."),
    ).toBeInTheDocument();
    expect(screen.getByText("79% confident")).toBeInTheDocument();
    await user.hover(screen.getByLabelText("About judge confidence"));
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "How certain the judge is of its verdict, independent",
    );
    expect(
      screen.getByRole("progressbar", { name: "Judge confidence" }),
    ).toHaveAttribute("aria-valuenow", "79");
    for (const label of [
      "Accuracy",
      "Completeness",
      "Instruction following",
      "Scope & clarity",
      "Tone",
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }

    await user.click(
      screen.getByRole("button", { name: "Hide explanation" }),
    );
    expect(
      screen.queryByText("The response did not address the request."),
    ).not.toBeInTheDocument();
  });
});
