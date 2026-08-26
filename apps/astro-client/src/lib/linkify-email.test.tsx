import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { linkifyEmail } from "./linkify-email";

describe("linkifyEmail", () => {
  it("turns an address inside a sentence into a mailto the reader can click", () => {
    render(
      <div>
        {linkifyEmail(
          "A spend threshold cannot exceed $1000 per month. Contact support@astropods.com about an enterprise plan to raise it.",
        )}
      </div>,
    );

    const link = screen.getByRole("link", { name: "support@astropods.com" });
    expect(link).toHaveAttribute("href", "mailto:support@astropods.com");
    expect(screen.getByText(/cannot exceed \$1000 per month/)).toBeInTheDocument();
    expect(screen.getByText(/about an enterprise plan to raise it/)).toBeInTheDocument();
  });

  it("leaves a message with no address alone", () => {
    render(<div>{linkifyEmail("The warning must be below the limit.")}</div>);

    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText("The warning must be below the limit.")).toBeInTheDocument();
  });
});
