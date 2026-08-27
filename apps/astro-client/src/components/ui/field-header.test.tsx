import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FieldHeader } from "./field-header";

describe("FieldHeader", () => {
  it("uses the shared title, helper, and control spacing", () => {
    render(
      <FieldHeader
        label="Deployment region"
        description="Select where this agent runs."
        htmlFor="region"
      />,
    );

    const label = screen.getByText("Deployment region");
    const description = screen.getByText("Select where this agent runs.");

    expect(label).toHaveAttribute("for", "region");
    expect(label).toHaveClass("mb-0");
    expect(description).toHaveClass("mt-0.5");
    expect(description.parentElement?.parentElement).toHaveClass("mb-3");
  });
});
