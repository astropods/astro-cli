import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ReviewQueuePrediction } from "@/lib/api";
import {
  PredictionVerdictIndicator,
  predictionVerdict,
} from "./PredictionVerdictIndicator";

function prediction(
  verdictScore: number,
  confidence = 80,
): ReviewQueuePrediction {
  return {
    verdict_score: verdictScore,
    confidence,
    explanation: "Prediction explanation",
    judge_version: "1",
    criteria: [],
  };
}

describe("PredictionVerdictIndicator", () => {
  it.each([
    { score: 0.25, label: "Good", verdict: "good" },
    { score: -0.25, label: "Bad", verdict: "bad" },
    { score: 0, label: "Not sure", verdict: "unknown" },
  ] as const)(
    "shows $label for a $score score",
    ({ score, label, verdict }) => {
      render(
        <PredictionVerdictIndicator
          prediction={prediction(score, 72)}
          status="completed"
        />,
      );

      expect(screen.getByText(label)).toBeInTheDocument();
      expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
      expect(predictionVerdict(score)).toBe(verdict);
    },
  );

  it("shows not judged when no prediction exists", () => {
    render(
      <PredictionVerdictIndicator
        prediction={null}
        status="not_requested"
      />,
    );

    expect(screen.getByText("Not judged")).toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it.each(["queued", "in_progress"] as const)(
    "shows a loading state while prediction status is %s",
    (status) => {
      render(
        <PredictionVerdictIndicator prediction={null} status={status} />,
      );

      expect(screen.getByText("Judging…")).toBeInTheDocument();
      expect(screen.queryByText("Not judged")).not.toBeInTheDocument();
    },
  );
});
