import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { TracePanelTitle, TraceNavButtons } from "./TracePanelHeader";

afterEach(cleanup);

const baseProps = {
  timestamp: "2026-07-07T00:00:00.000Z",
  traceId: "trace-abc",
};

describe("TracePanelTitle", () => {
  it("renders a copy-link button and calls onShare when clicked", () => {
    const onShare = vi.fn();
    render(<TracePanelTitle {...baseProps} onShare={onShare} />);

    fireEvent.click(screen.getByRole("button", { name: "Copy link to this trace" }));
    expect(onShare).toHaveBeenCalledTimes(1);
  });

  it("keeps the button discoverable while showing the copied state", () => {
    render(<TracePanelTitle {...baseProps} onShare={() => {}} shareCopied />);
    expect(
      screen.getByRole("button", { name: "Copy link to this trace" }),
    ).toBeInTheDocument();
  });

  it("omits the copy-link button when onShare is not provided", () => {
    render(<TracePanelTitle {...baseProps} />);
    expect(
      screen.queryByRole("button", { name: "Copy link to this trace" }),
    ).toBeNull();
  });

  it("always renders a copy-trace-id button next to the id", () => {
    render(<TracePanelTitle {...baseProps} />);
    expect(
      screen.getByRole("button", { name: "Copy trace ID" }),
    ).toBeInTheDocument();
  });
});

describe("TraceNavButtons", () => {
  it("navigates prev/next and respects disabled bounds", () => {
    const onNavigate = vi.fn();
    render(<TraceNavButtons canGoPrev={false} canGoNext onNavigate={onNavigate} />);

    expect(screen.getByRole("button", { name: "Previous trace" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Next trace" }));
    expect(onNavigate).toHaveBeenCalledWith("next");
  });

  it("renders nothing without an onNavigate handler", () => {
    const { container } = render(<TraceNavButtons />);
    expect(container).toBeEmptyDOMElement();
  });
});
