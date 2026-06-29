import { render, screen, cleanup } from "@testing-library/react";
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
    ...overrides,
  };
}

describe("DatasetGradeSidebar", () => {
  it("renders the grade letter and headline for an A dataset", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    expect(screen.getByText(/dataset reliability/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/grade a/i)).toBeInTheDocument();
    expect(screen.getAllByText(/dataset looks healthy/i).length).toBeGreaterThan(0);
  });

  it("maps headlines from the grade letter", () => {
    const cases: Array<[string, RegExp]> = [
      ["A", /dataset looks healthy/i],
      ["B", /improve your dataset/i],
      ["C", /needs more coverage/i],
      ["D", /needs more coverage/i],
      ["F", /needs more coverage/i],
    ];
    for (const [grade, label] of cases) {
      cleanup();
      render(
        <DatasetGradeSidebar
          summary={summary({ grade, next_grade: grade === "A" ? "" : "A" })}
        />,
      );
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
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
          next_grade_progress: 0,
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

  it("hides the progress bar when already at A", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    expect(screen.queryByText(/more to/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/keep grading to/i)).not.toBeInTheDocument();
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
          next_grade_progress: 0.28,
          cases_to_next_grade: 21,
        })}
      />,
    );
    expect(screen.getByText(/at least 21 mixed labels to D/)).toBeInTheDocument();
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
          next_grade_progress: 0.67,
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
          next_grade_progress: 0.5,
        })}
      />,
    );
    expect(screen.getAllByText(/add failure cases/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/only 2% of traces are labeled bad/i)).toBeInTheDocument();
  });

  it("shows healthy dataset guidance for an A with healthy coverage", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    const guidanceTitle = screen.getAllByText(/dataset looks healthy/i)[1];
    expect(guidanceTitle).toBeInTheDocument();
    expect(guidanceTitle.closest('[data-slot="card"]')).toBeInTheDocument();
    expect(screen.getByText(/this dataset is a reliable signal/i)).toBeInTheDocument();
  });

  it("places guidance after the composition summary", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    const composition = screen.getByText(/composition · 100/i);
    const guidanceTitle = screen.getAllByText(/dataset looks healthy/i)[1];

    expect(
      composition.compareDocumentPosition(guidanceTitle) &
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
          next_grade_progress: 0.4,
        })}
      />,
    );
    expect(screen.getByText(/reduce noise/i)).toBeInTheDocument();
    expect(screen.getByText(/40% of traces are labeled bad/i)).toBeInTheDocument();
    expect(
      screen.getByText(/add good responses or remove bad labels that don't reflect real failures/i),
    ).toBeInTheDocument();
  });

  it("shows composition counts in the legend", () => {
    render(<DatasetGradeSidebar summary={summary({ good_count: 30, bad_count: 12, item_count: 42 })} />);
    expect(screen.getByText(/composition · 42/i)).toBeInTheDocument();
    expect(screen.getByText(/30 good/)).toBeInTheDocument();
    expect(screen.getByText(/12 bad/)).toBeInTheDocument();
  });

  it("formats large composition counts with locale separators", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({ item_count: 12345, good_count: 12000, bad_count: 345 })}
      />,
    );
    expect(screen.getByText(/composition · 12,345/i)).toBeInTheDocument();
    expect(screen.getByText(/12,000 good/)).toBeInTheDocument();
  });
});
