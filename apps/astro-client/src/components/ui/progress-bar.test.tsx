import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ProgressBar } from "./progress-bar";

describe("ProgressBar", () => {
  it("renders value as a percentage of max", () => {
    render(<ProgressBar aria-label="Usage" value={3} max={4} />);

    const progress = screen.getByRole("progressbar", { name: "Usage" });
    expect(progress).toHaveAttribute("aria-valuenow", "3");
    expect(progress).toHaveAttribute("aria-valuemax", "4");
    expect(progress.firstElementChild).toHaveStyle({ width: "75%" });
  });

  it("clamps values to the available range", () => {
    const { rerender } = render(
      <ProgressBar aria-label="Usage" value={120} />,
    );

    expect(screen.getByRole("progressbar").firstElementChild).toHaveStyle({
      width: "100%",
    });

    rerender(<ProgressBar aria-label="Usage" value={-10} />);
    expect(screen.getByRole("progressbar").firstElementChild).toHaveStyle({
      width: "0%",
    });
  });
});
