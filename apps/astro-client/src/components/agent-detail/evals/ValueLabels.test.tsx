import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ValueLabels } from "./ValueLabels";

afterEach(cleanup);

const itemNoun = { singular: "value", plural: "values" };

describe("ValueLabels", () => {
  it("renders a placeholder when there are no labels", () => {
    render(<ValueLabels labels={[]} itemNoun={itemNoun} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("shows the first label and an overflow count for the rest", () => {
    render(
      <ValueLabels
        labels={["Exposed PII: False", "User sentiment: Positive", "Tone: True"]}
        itemNoun={itemNoun}
      />,
    );
    expect(screen.getByText("Exposed PII: False")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /show 3 values/i }),
    ).toHaveTextContent("+2");
    expect(screen.queryByText("User sentiment: Positive")).not.toBeInTheDocument();
  });

  it("reveals all labels on hover", () => {
    render(
      <ValueLabels
        labels={["Exposed PII: False", "User sentiment: Positive"]}
        itemNoun={itemNoun}
      />,
    );
    fireEvent.mouseEnter(screen.getByRole("button", { name: /show 2 values/i }));
    expect(screen.getAllByText("Exposed PII: False").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("User sentiment: Positive")).toBeInTheDocument();
  });
});
