import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FieldHeader } from "./field-header";

describe("FieldHeader", () => {
  it("associates its label with the control and renders helper text", () => {
    render(
      <>
        <FieldHeader
          label="Deployment region"
          description="Select where this agent runs."
          htmlFor="region"
        />
        <input id="region" />
      </>,
    );

    expect(screen.getByRole("textbox", { name: "Deployment region" })).toBeInTheDocument();
    expect(screen.getByText("Select where this agent runs.")).toBeInTheDocument();
  });
});
