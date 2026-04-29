import { describe, it, expect, afterEach } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { Card } from "./card";

afterEach(cleanup);

describe("<Card>", () => {
  it("renders bordered chrome on the semantic card token", () => {
    render(<Card data-testid="card">content</Card>);
    const el = screen.getByTestId("card");
    expect(el.tagName).toBe("DIV");
    expect(el).toHaveClass("bg-card");
    expect(el).toHaveClass("text-card-foreground");
    expect(el).toHaveClass("border");
    expect(el).toHaveClass("border-border");
    expect(el).toHaveClass("rounded-[10px]");
    expect(el).toHaveAttribute("data-slot", "card");
  });

  it("merges className with the base chrome", () => {
    render(
      <Card data-testid="card" className="p-[12px_14px] ring-2">
        content
      </Card>,
    );
    const el = screen.getByTestId("card");
    expect(el).toHaveClass("bg-card");
    expect(el).toHaveClass("p-[12px_14px]");
    expect(el).toHaveClass("ring-2");
  });

  it("forwards arbitrary div props", () => {
    render(
      <Card data-testid="card" role="region" aria-label="metrics">
        content
      </Card>,
    );
    const el = screen.getByTestId("card");
    expect(el).toHaveAttribute("role", "region");
    expect(el).toHaveAttribute("aria-label", "metrics");
  });
});
