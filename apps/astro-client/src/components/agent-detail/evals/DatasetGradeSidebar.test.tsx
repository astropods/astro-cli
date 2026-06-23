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
    ...overrides,
  };
}

describe("DatasetGradeSidebar", () => {
  it("renders the grade letter and headline for an A baseline", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    expect(screen.getByLabelText(/grade a/i)).toBeInTheDocument();
    expect(screen.getByText(/strong baseline/i)).toBeInTheDocument();
  });

  it("maps headlines from the grade letter", () => {
    const cases: Array<[string, RegExp]> = [
      ["A", /strong baseline/i],
      ["B", /solid baseline/i],
      ["C", /needs more coverage/i],
      ["D", /weak baseline/i],
      ["F", /weak baseline/i],
    ];
    for (const [grade, label] of cases) {
      cleanup();
      render(
        <DatasetGradeSidebar
          summary={summary({ grade, next_grade: grade === "A" ? "" : "A" })}
        />,
      );
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it("renders empty baseline when there are no items", () => {
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
    expect(screen.getByText(/no baseline yet/i)).toBeInTheDocument();
  });

  it("hides the progress bar when already at A", () => {
    render(<DatasetGradeSidebar summary={summary()} />);
    expect(screen.queryByText(/% to /i)).not.toBeInTheDocument();
  });

  it("shows '{N}% to {next_grade}' when there is a next grade", () => {
    render(
      <DatasetGradeSidebar
        summary={summary({
          grade: "F",
          next_grade: "D",
          next_grade_progress: 0.42,
        })}
      />,
    );
    expect(screen.getByText(/42% to D/)).toBeInTheDocument();
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
