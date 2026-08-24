import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { EvaluationRun, EvaluationRunStatus } from "@/lib/api";
import { EvaluationStatusIndicator } from "./EvaluationStatusIndicator";

function run(status: EvaluationRunStatus): EvaluationRun {
  return { status, error: null };
}

describe("EvaluationStatusIndicator", () => {
  it("marks a completed run as evaluated", () => {
    render(<EvaluationStatusIndicator run={run("completed")} />);

    expect(screen.getByLabelText("Evaluated")).toHaveClass("text-primary");
  });

  it("shows nothing for a trace with no run", () => {
    const { container } = render(<EvaluationStatusIndicator run={null} />);

    expect(container).toBeEmptyDOMElement();
  });

  it.each(["queued", "in_progress"] as const)(
    "spins while the run is %s",
    (status) => {
      render(<EvaluationStatusIndicator run={run(status)} />);

      expect(screen.getByLabelText("Evaluating")).toHaveClass(
        "dp-spin",
        "text-muted-foreground",
      );
    },
  );

  it("marks a failed run as having no result", () => {
    render(<EvaluationStatusIndicator run={run("failed")} />);

    expect(screen.getByLabelText("Couldn’t evaluate")).toHaveClass(
      "text-muted-foreground",
    );
  });
});
