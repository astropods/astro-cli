import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContentSection } from "./ContentSection";

afterEach(cleanup);
afterEach(() => vi.restoreAllMocks());

describe("ContentSection", () => {
  it("uses a visible border for trace content cards", () => {
    render(<ContentSection label="User" content="hello" />);
    expect(screen.getByText("User").closest("section")).toHaveClass(
      "border-border/70",
    );
  });

  it("renders optional header metadata", () => {
    render(
      <ContentSection
        label="User"
        content="hello"
        headerMeta={<span>14:08</span>}
      />,
    );

    expect(screen.getByText("14:08")).toBeInTheDocument();
  });

  it("applies an optional content scroll class", () => {
    render(
      <ContentSection
        label="User"
        content="hello"
        contentClassName="max-h-72 overflow-y-auto"
      />,
    );

    expect(screen.getByText("hello").closest(".max-h-72")).toHaveClass(
      "overflow-y-auto",
    );
  });

  it("renders a stable corner resize affordance for content", () => {
    render(<ContentSection label="User" content="hello" resizableContent />);
    const resizeButton = screen.getByRole("button", {
      name: /resize user content/i,
    });
    expect(resizeButton).toContainElement(document.querySelector("[data-resize-corner]"));
    expect(resizeButton).toHaveClass("touch-none");
    expect(screen.getByText("hello").closest(".resize-y")).not.toBeInTheDocument();
  });

  it("resizes content with arrow keys on the resize affordance", () => {
    render(<ContentSection label="User" content="hello" resizableContent />);
    const resizeButton = screen.getByRole("button", {
      name: /resize user content/i,
    });
    const contentBody = screen.getByText("hello").closest(".min-h-32");
    if (!(contentBody instanceof HTMLElement)) {
      throw new Error("Resizable content body not found");
    }

    expect(contentBody).toHaveStyle({ height: "128px" });

    fireEvent.keyDown(resizeButton, { key: "ArrowDown" });
    expect(contentBody).toHaveStyle({ height: "152px" });

    fireEvent.keyDown(resizeButton, { key: "ArrowUp" });
    expect(contentBody).toHaveStyle({ height: "128px" });
  });

  it("keeps the resize affordance out of non-resizable content", () => {
    render(<ContentSection label="User" content="hello" />);
    expect(
      screen.queryByRole("button", { name: /resize user content/i }),
    ).not.toBeInTheDocument();
    expect(
      document.querySelector("[data-resize-corner]"),
    ).not.toBeInTheDocument();
  });

  it("keeps optional content scroll classes on the resizable scroll body", () => {
    render(
      <ContentSection
        label="User"
        content="hello"
        contentClassName="max-h-72 overflow-y-auto"
        resizableContent
      />,
    );

    expect(screen.getByText("hello").closest(".max-h-72")).toHaveClass(
      "overflow-y-auto",
    );
  });
});
