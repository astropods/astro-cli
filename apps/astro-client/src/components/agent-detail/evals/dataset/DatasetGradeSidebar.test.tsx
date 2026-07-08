import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { DatasetGradeSidebar } from "./DatasetGradeSidebar";
import type { EvalDatasetResponse } from "@/lib/api";

afterEach(cleanup);

function summary(overrides: Partial<EvalDatasetResponse> = {}): EvalDatasetResponse {
  return {
    dataset_name: "dep-test",
    item_count: 100,
    good_count: 90,
    bad_count: 10,
    grade: "A",
    next_grade: "",
    next_grade_progress: 1,
    cases_to_next_grade: null,
    criteria_counts: [],
    ...overrides,
  };
}

describe("DatasetGradeSidebar", () => {
  it("renders the baseline grade header and grade letter for an A dataset", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    expect(screen.getByText(/baseline grade/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/grade a/i)).toBeInTheDocument();
    expect(screen.getByText(/dataset looks healthy/i)).toBeInTheDocument();
  });

  it("renders empty guidance when there are no items", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          item_count: 0,
          good_count: 0,
          bad_count: 0,
          grade: "—",
          next_grade: "",
        })}
      />,
    );
    expect(screen.getAllByText(/start grading/i)).toHaveLength(1);
    expect(
      screen.getByText(
        /label recent traces as good or bad\. these labels determine how reliable this dataset is\./i,
      ),
    ).toBeInTheDocument();
  });

  it("shows the top-grade ring caption and no progress percent when already at A", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    expect(screen.getByText(/top grade/i)).toBeInTheDocument();
    expect(screen.queryByText(/% to/i)).not.toBeInTheDocument();
  });

  it("shows the ring progress percent toward the next grade", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          grade: "C",
          next_grade: "B",
          next_grade_progress: 0.67,
        })}
      />,
    );
    expect(screen.getByText("67% to B")).toBeInTheDocument();
  });

  it("shows how many more judged cases are needed for the next grade", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          item_count: 3,
          good_count: 3,
          bad_count: 0,
          grade: "F",
          next_grade: "D",
          cases_to_next_grade: 21,
        })}
      />,
    );
    expect(
      screen.getByText(/label 21 or more traces to raise this grade to a D/i),
    ).toBeInTheDocument();
  });

  it("shows low-volume guidance with the remaining case count", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          item_count: 14,
          good_count: 11,
          bad_count: 3,
          grade: "C",
          next_grade: "B",
        })}
      />,
    );
    expect(screen.getByText(/grade more cases/i)).toBeInTheDocument();
    expect(
      screen.getByText(
        /more labels make the dataset score more reliable\. make sure to include some bad cases\./i,
      ),
    ).toBeInTheDocument();
  });

  it("shows failure-case guidance when the dataset has too few bad cases", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          item_count: 100,
          good_count: 98,
          bad_count: 2,
          grade: "B",
          next_grade: "A",
        })}
      />,
    );
    expect(screen.getAllByText(/add failure cases/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/only 2% of traces are labeled bad/i)).toBeInTheDocument();
  });

  it("shows healthy dataset guidance for an A with healthy coverage", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    const guidanceTitle = screen.getByText(/dataset looks healthy/i);
    expect(guidanceTitle).toBeInTheDocument();
    expect(guidanceTitle.closest('[data-slot="card"]')).toBeInTheDocument();
    expect(screen.getByText(/this dataset is a reliable signal/i)).toBeInTheDocument();
  });

  it("places guidance after the baseline grade header", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    const header = screen.getByText(/baseline grade/i);
    const guidanceTitle = screen.getByText(/dataset looks healthy/i);

    expect(
      header.compareDocumentPosition(guidanceTitle) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("shows noise-reduction guidance when too many traces are labeled bad", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          item_count: 100,
          good_count: 60,
          bad_count: 40,
          grade: "C",
          next_grade: "B",
        })}
      />,
    );
    expect(screen.getByText(/reduce noise/i)).toBeInTheDocument();
    expect(screen.getByText(/40% of traces are labeled bad/i)).toBeInTheDocument();
    expect(
      screen.getByText(/add good responses or remove bad labels that don't reflect real failures/i),
    ).toBeInTheDocument();
  });

  it("omits the reasons section when no criterion has any count", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          criteria_counts: [
            { dimension_key: "accuracy", good_count: 0, bad_count: 0 },
          ],
        })}
      />,
    );
    expect(screen.queryByText(/^reasons$/i)).not.toBeInTheDocument();
  });

  it("defaults to bad reasons, sorted most frequent first, skipping zero counts", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          criteria_counts: [
            { dimension_key: "accuracy", good_count: 1, bad_count: 2 },
            { dimension_key: "completeness", good_count: 0, bad_count: 5 },
            { dimension_key: "tone", good_count: 4, bad_count: 0 },
          ],
        })}
      />,
    );
    expect(screen.getByText(/^reasons$/i)).toBeInTheDocument();
    expect(screen.getByText("Incomplete")).toBeInTheDocument();
    expect(screen.getByText("Hallucination")).toBeInTheDocument();
    // tone has no bad count, so it is not listed in the default bad view
    expect(screen.queryByText("Appropriate tone")).not.toBeInTheDocument();

    // sorted by bad count descending: Incomplete (5) before Hallucination (2)
    const incomplete = screen.getByText("Incomplete");
    const hallucination = screen.getByText("Hallucination");
    expect(
      incomplete.compareDocumentPosition(hallucination) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("toggles to good reasons, sorted most frequent first, skipping zero counts", async () => {
    const user = userEvent.setup();
    render(
      <DatasetGradeSidebar
        summary={summary({
          criteria_counts: [
            { dimension_key: "accuracy", good_count: 2, bad_count: 1 },
            { dimension_key: "completeness", good_count: 6, bad_count: 0 },
            { dimension_key: "tone", good_count: 0, bad_count: 3 },
          ],
        })}
      />,
    );

    await user.click(screen.getByRole("radio", { name: /good/i }));

    expect(screen.getByText("Complete")).toBeInTheDocument();
    expect(screen.getByText("Correct info")).toBeInTheDocument();
    // tone has no good count, so it is not listed in the good view
    expect(screen.queryByText("Appropriate tone")).not.toBeInTheDocument();

    // sorted by good count descending: Complete (6) before Correct info (2)
    const complete = screen.getByText("Complete");
    const correct = screen.getByText("Correct info");
    expect(
      complete.compareDocumentPosition(correct) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("defaults to the good view when there are good reasons but no bad ones", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          criteria_counts: [
            { dimension_key: "accuracy", good_count: 3, bad_count: 0 },
          ],
        })}
      />,
    );
    expect(screen.getByText(/^reasons$/i)).toBeInTheDocument();
    expect(screen.getByText("Correct info")).toBeInTheDocument();
  });

  it("shows an empty message when the selected verdict has no reasons", async () => {
    const user = userEvent.setup();
    render(
      <DatasetGradeSidebar
        summary={summary({
          criteria_counts: [
            { dimension_key: "accuracy", good_count: 3, bad_count: 0 },
          ],
        })}
      />,
    );
    // defaults to good; switching to bad has nothing to show
    await user.click(screen.getByRole("radio", { name: /bad/i }));
    expect(screen.getByText(/no bad reasons labeled yet/i)).toBeInTheDocument();
  });

});
