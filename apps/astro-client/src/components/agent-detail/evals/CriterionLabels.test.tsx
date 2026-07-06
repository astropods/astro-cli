import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { CriterionLabels } from "./CriterionLabels";
import type { JudgmentCriterion } from "@/lib/api";

afterEach(cleanup);

const c = (dimension_key: string, value: number): JudgmentCriterion => ({
  dimension_key,
  value,
});

describe("CriterionLabels", () => {
  it("renders a placeholder when there are no criteria", () => {
    render(<CriterionLabels criteria={[]} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders a placeholder when every criterion has an unknown dimension", () => {
    render(<CriterionLabels criteria={[c("bogus", 1)]} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders a single label without an overflow chip", () => {
    render(<CriterionLabels criteria={[c("accuracy", -1)]} />);
    expect(screen.getByText("Hallucination")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("shows only the first label plus a +N overflow chip, hiding the rest", () => {
    render(
      <CriterionLabels
        criteria={[c("accuracy", 1), c("completeness", -1), c("tone", -1)]}
      />,
    );
    expect(screen.getByText("Correct info")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /show 3 reasons/i }),
    ).toHaveTextContent("+2");
    expect(screen.queryByText("Incomplete")).not.toBeInTheDocument();
    expect(screen.queryByText("Inappropriate tone")).not.toBeInTheDocument();
  });

  it("excludes unknown dimensions from the overflow count", () => {
    render(
      <CriterionLabels
        criteria={[c("accuracy", 1), c("bogus", -1), c("tone", -1)]}
      />,
    );
    expect(
      screen.getByRole("button", { name: /show 2 reasons/i }),
    ).toHaveTextContent("+1");
  });

  it("reveals all labels on hover", () => {
    render(
      <CriterionLabels
        criteria={[c("accuracy", 1), c("completeness", -1), c("tone", -1)]}
      />,
    );
    fireEvent.mouseEnter(screen.getByRole("button", { name: /show 3 reasons/i }));
    // The first label now appears both inline and inside the revealed panel.
    expect(screen.getAllByText("Correct info").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("Incomplete")).toBeInTheDocument();
    expect(screen.getByText("Inappropriate tone")).toBeInTheDocument();
  });
});
