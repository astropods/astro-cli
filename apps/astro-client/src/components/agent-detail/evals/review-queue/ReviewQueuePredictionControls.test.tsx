import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
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
        isPending={false}
        activeVerdict={null}
        showError={false}
        explanationOpen={open}
        onExplanationOpenChange={setOpen}
        onSelect={vi.fn()}
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

  it("agrees with the prediction or selects an alternative verdict", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(
      <ReviewQueuePredictionControls
        prediction={prediction}
        isPending={false}
        activeVerdict={null}
        showError={false}
        explanationOpen={false}
        onExplanationOpenChange={vi.fn()}
        onSelect={onSelect}
      />,
    );

    const disagreeButton = screen.getByRole("button", { name: "Disagree" });
    const agreeButton = screen.getByRole("button", { name: "Agree with judge" });
    expect(disagreeButton.compareDocumentPosition(agreeButton)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(disagreeButton).toHaveAttribute("data-variant", "outline");
    expect(agreeButton).toHaveAttribute("data-variant", "default");

    await user.click(agreeButton);
    expect(onSelect).toHaveBeenLastCalledWith(
      "bad",
      expect.any(HTMLElement),
      true,
    );

    await user.click(disagreeButton);
    expect(screen.getByText("Mark as instead")).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /Good/ })).toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /Not sure/ }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("menuitem", { name: /Bad/ }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("menuitem", { name: /Good/ }));
    expect(onSelect).toHaveBeenLastCalledWith(
      "good",
      expect.any(HTMLElement),
      false,
    );
  });
});
