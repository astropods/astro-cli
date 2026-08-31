import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReviewQueueEvaluationResults } from "./ReviewQueueEvaluationResults";
import type { EvaluationRow } from "./evaluation-rows";

afterEach(cleanup);

function row(overrides: Partial<EvaluationRow> = {}): EvaluationRow {
  return {
    key: "exposed_pii",
    label: "Exposed PII",
    description: "Flags personal data in the output.",
    evaluated: true,
    explanation: "No personal data appeared in the answer.",
    output: { type: "boolean" },
    confidence: 82,
    ...overrides,
  };
}

function renderResults(
  props: Partial<
    React.ComponentProps<typeof ReviewQueueEvaluationResults>
  > = {},
) {
  const onChange = vi.fn();
  const view = render(
    <ReviewQueueEvaluationResults
      rows={[row()]}
      values={new Map([["exposed_pii", false]])}
      editedKeys={new Set()}
      scored
      onChange={onChange}
      {...props}
    />,
  );
  return { ...view, onChange };
}

describe("ReviewQueueEvaluationResults", () => {
  it("shows the evaluator, its reasoning, value, and confidence", () => {
    renderResults();

    expect(screen.getByText("Exposed PII")).toBeInTheDocument();
    expect(
      screen.getByText("No personal data appeared in the answer."),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Exposed PII" }),
    ).toHaveTextContent("False");
    expect(screen.getByText("82%")).toBeInTheDocument();
  });

  it("offers the definition beside the evaluator name", () => {
    renderResults();

    expect(screen.getByLabelText("About Exposed PII")).toBeInTheDocument();
  });

  it("omits the definition when the run used an unresolvable set", () => {
    renderResults({
      rows: [row({ key: "retired_check", label: "retired_check", description: undefined })],
      values: new Map(),
    });

    expect(screen.getByText("retired_check")).toBeInTheDocument();
    expect(screen.queryByLabelText(/^About /)).not.toBeInTheDocument();
  });

  it("titles an enum value", () => {
    renderResults({
      rows: [
        row({
          key: "claim_grounding",
          label: "Claim grounding",
          output: { type: "enum", options: ["grounded", "no_claims"] },
        }),
      ],
      values: new Map([["claim_grounding", "no_claims"]]),
    });

    expect(
      screen.getByRole("combobox", { name: "Claim grounding" }),
    ).toHaveTextContent("No claims");
  });

  it("records the value the reviewer picks", async () => {
    const user = userEvent.setup();
    const { onChange } = renderResults();

    await user.click(screen.getByRole("combobox", { name: "Exposed PII" }));
    await user.click(screen.getByRole("option", { name: "True" }));

    expect(onChange).toHaveBeenCalledWith("exposed_pii", true);
  });

  it("marks a value the reviewer changed", () => {
    renderResults({ editedKeys: new Set(["exposed_pii"]) });

    expect(screen.getByText("Updated")).toBeInTheDocument();
  });

  it("leaves an unscored row unmarked when the reviewer fills it in", () => {
    renderResults({
      rows: [row({ evaluated: false, explanation: null, confidence: null })],
      editedKeys: new Set(["exposed_pii"]),
    });

    expect(screen.queryByText("Updated")).not.toBeInTheDocument();
  });

  it("stands in for a row still waiting on its result", () => {
    renderResults({
      rows: [row({ evaluated: false, explanation: null, confidence: null })],
      loading: true,
    });

    expect(screen.getByText("Pending")).toBeInTheDocument();
    expect(
      screen.queryByRole("combobox", { name: "Exposed PII" }),
    ).not.toBeInTheDocument();
  });

  it("dashes the confidence an evaluator never produced", () => {
    renderResults({
      rows: [row({ evaluated: false, explanation: null, confidence: null })],
    });

    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("drops the confidence column when nothing scored the trace", () => {
    renderResults({
      rows: [row({ evaluated: false, explanation: null, confidence: null })],
      scored: false,
    });

    expect(screen.queryByText("Confidence")).not.toBeInTheDocument();
    expect(screen.getByText("Result")).toBeInTheDocument();
  });

  it("renders nothing without an evaluation set to list", () => {
    const { container } = renderResults({ rows: [] });

    expect(container).toBeEmptyDOMElement();
  });
});
