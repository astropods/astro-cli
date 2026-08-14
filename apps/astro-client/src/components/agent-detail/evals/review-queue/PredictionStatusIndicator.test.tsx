import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ReviewQueuePrediction } from "@/lib/api";
import { PredictionStatusIndicator } from "./PredictionStatusIndicator";

function prediction(verdictScore: number): ReviewQueuePrediction {
  return {
    verdict_score: verdictScore,
    confidence: 80,
    explanation: "Prediction explanation",
    judge_version: "1",
    criteria: [],
  };
}

describe("PredictionStatusIndicator", () => {
  it.each([0.25, -0.25, 0])("shows judged for a %s score", (score) => {
    render(
      <PredictionStatusIndicator
        prediction={prediction(score)}
        status="completed"
      />,
    );

    const judged = screen.getByText("Judged");
    expect(judged).toBeInTheDocument();
    expect(judged.parentElement).toHaveClass("!rounded-sm");
    expect(judged.parentElement).toHaveStyle({
      background: "transparent",
      color: "var(--sb-primary-fg, var(--primary))",
    });
  });

  it("shows not judged when no prediction exists", () => {
    render(
      <PredictionStatusIndicator prediction={null} status="not_requested" />,
    );

    expect(screen.getByText("Not judged")).toBeInTheDocument();
    expect(screen.getByText("Not judged").parentElement).toHaveClass(
      "!rounded-sm",
      "border-dashed",
    );
    expect(screen.getByText("Not judged").parentElement).toHaveStyle({
      background: "transparent",
    });
  });

  it.each(["queued", "in_progress"] as const)(
    "shows a loading state while prediction status is %s",
    (status) => {
      render(<PredictionStatusIndicator prediction={null} status={status} />);

      expect(screen.getByText("Judging…")).toBeInTheDocument();
      expect(screen.queryByText("Not judged")).not.toBeInTheDocument();
    },
  );

  it("shows a warning when prediction generation failed", () => {
    render(<PredictionStatusIndicator prediction={null} status="failed" />);

    expect(screen.getByText("Couldn’t judge")).toBeInTheDocument();
    expect(screen.getByText("Couldn’t judge").parentElement).toHaveStyle({
      background: "transparent",
    });
    expect(screen.queryByText("Not judged")).not.toBeInTheDocument();
    expect(screen.queryByText("Judging…")).not.toBeInTheDocument();
  });
});
