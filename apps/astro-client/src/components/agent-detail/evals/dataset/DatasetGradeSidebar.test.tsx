import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { EvalDatasetResponse } from "@/lib/api";
import { DatasetGradeSidebar } from "./DatasetGradeSidebar";

afterEach(cleanup);

function summary(
  criteria_counts: EvalDatasetResponse["criteria_counts"] = [],
): EvalDatasetResponse {
  return {
    dataset_name: "dep-test",
    item_count: 10,
    good_count: 8,
    bad_count: 2,
    grade: "A",
    next_grade: "",
    next_grade_progress: 1,
    cases_to_next_grade: null,
    criteria_counts,
  };
}

describe("DatasetGradeSidebar", () => {
  it("shows a neutral empty state without criterion values", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    expect(
      screen.getByRole("heading", { level: 3, name: "Evaluation criteria" }),
    ).toHaveClass("text-heading-3");
    expect(
      screen.getByText("Evaluations recorded for traces in this dataset."),
    ).toBeInTheDocument();
    expect(screen.getByText("No criteria values recorded yet.")).toBeInTheDocument();
    expect(screen.queryByText(/grade/i)).not.toBeInTheDocument();
  });

  it("renders criteria in definition order with independent percentages", () => {
    render(
      <DatasetGradeSidebar
        summary={summary([
          { dimension_key: "tone", good_count: 1, bad_count: 3 },
          { dimension_key: "accuracy", good_count: 3, bad_count: 1 },
        ])}
      />,
    );

    const headings = screen.getAllByText(/Accuracy|Completeness|Instruction following|Scope & clarity|Tone/);
    expect(headings.map((heading) => heading.textContent)).toEqual([
      "Accuracy",
      "Tone",
    ]);
    expect(screen.getByText("3:1")).toBeInTheDocument();
    expect(screen.getByText("1:3")).toBeInTheDocument();
    expect(screen.queryByText("0:0")).not.toBeInTheDocument();
    expect(
      screen.getByRole("progressbar", {
        name: "Accuracy positive distribution",
      }),
    ).toHaveAttribute("aria-valuenow", "3");
    expect(
      screen.getByRole("progressbar", {
        name: "Accuracy positive distribution",
      }),
    ).toHaveAttribute("aria-valuemax", "4");
  });

  it("reduces criterion counts to a ratio", () => {
    render(
      <DatasetGradeSidebar
        summary={summary([
          { dimension_key: "accuracy", good_count: 50, bad_count: 2 },
        ])}
      />,
    );

    expect(screen.getByText("25:1")).toBeInTheDocument();
  });

  it("shows each criterion's positive definition in a tooltip", async () => {
    const user = userEvent.setup();
    render(
      <DatasetGradeSidebar
        summary={summary([
          { dimension_key: "accuracy", good_count: 1, bad_count: 0 },
        ])}
      />,
    );

    await user.hover(screen.getByLabelText("About Accuracy"));
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "The answer was factually accurate, with no invented facts, citations, or capabilities.",
    );
  });
});
