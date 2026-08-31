import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { EvalDatasetResponse } from "@/lib/api";
import { DatasetGradeSidebar } from "./DatasetGradeSidebar";

afterEach(cleanup);

function summary(
  evaluators: EvalDatasetResponse["evaluators"] = [],
): EvalDatasetResponse {
  return { dataset_name: "dep-test", item_count: 10, evaluators };
}

describe("DatasetGradeSidebar", () => {
  it("shows a neutral empty state without evaluator values", () => {
    render(<DatasetGradeSidebar summary={summary()} />);

    expect(
      screen.getByRole("heading", { level: 3, name: "Dataset overview" }),
    ).toHaveClass("text-heading-4");
    expect(
      screen.getByText("No evaluator values recorded yet."),
    ).toBeInTheDocument();
  });

  it("totals each evaluator and keeps the order the server sent", () => {
    render(
      <DatasetGradeSidebar
        summary={summary([
          {
            key: "claim_grounding",
            label: "Claim grounding",
            distribution: [
              { value: "grounded", count: 3 },
              { value: "no_claims", count: 5 },
            ],
          },
          {
            key: "exposed_pii",
            label: "Exposed PII",
            distribution: [{ value: false, count: 4 }],
          },
        ])}
      />,
    );

    const labels = screen
      .getAllByText(/Exposed PII|Claim grounding/)
      .map((node) => node.textContent);
    expect(labels).toEqual(["Claim grounding", "Exposed PII"]);
    expect(screen.getByText("8")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
  });

  it("labels an evaluator the set no longer defines with its key", () => {
    render(
      <DatasetGradeSidebar
        summary={summary([
          {
            key: "retired_check",
            label: "retired_check",
            distribution: [{ value: true, count: 1 }],
          },
        ])}
      />,
    );

    expect(screen.getByText("retired_check")).toBeInTheDocument();
  });

  it("ranks the values behind an evaluator by how many items hold them", async () => {
    const user = userEvent.setup();
    render(
      <DatasetGradeSidebar
        summary={summary([
          {
            key: "claim_grounding",
            label: "Claim grounding",
            distribution: [
              { value: "grounded", count: 3 },
              { value: "unsupported", count: 1 },
              { value: "no_claims", count: 20 },
            ],
          },
        ])}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Claim grounding/ }));

    const values = screen
      .getAllByText(/Grounded|Unsupported|No claims/)
      .map((node) => node.textContent);
    expect(values).toEqual(["No claims", "Grounded", "Unsupported"]);
  });

  it("keeps each evaluator's values collapsed until asked", async () => {
    const user = userEvent.setup();
    render(
      <DatasetGradeSidebar
        summary={summary([
          {
            key: "exposed_pii",
            label: "Exposed PII",
            distribution: [{ value: false, count: 2 }],
          },
        ])}
      />,
    );

    expect(screen.queryByText("False")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /Exposed PII/ }));

    expect(screen.getByText("False")).toBeInTheDocument();
  });
});
