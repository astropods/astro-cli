import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ContentSection } from "./ContentSection";

afterEach(cleanup);

describe("ContentSection", () => {
  it("uses a visible border for trace content cards", () => {
    render(<ContentSection label="User" content="hello" />);
    expect(screen.getByText("User").closest("section")).toHaveClass(
      "border-border/70",
    );
  });
});
