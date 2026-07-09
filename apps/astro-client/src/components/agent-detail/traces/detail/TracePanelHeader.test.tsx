import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { TracePanelHeader } from "./TracePanelHeader";

afterEach(cleanup);

const baseProps = {
  timestamp: "2026-07-07T00:00:00.000Z",
  traceId: "trace-abc",
  onClose: () => {},
};

describe("TracePanelHeader share button", () => {
  it("renders a copy-link button and calls onShare when clicked", () => {
    const onShare = vi.fn();
    render(<TracePanelHeader {...baseProps} onShare={onShare} />);

    fireEvent.click(screen.getByRole("button", { name: "Copy link to this trace" }));
    expect(onShare).toHaveBeenCalledTimes(1);
  });

  it("keeps the button discoverable while showing the copied state", () => {
    render(<TracePanelHeader {...baseProps} onShare={() => {}} shareCopied />);
    expect(
      screen.getByRole("button", { name: "Copy link to this trace" }),
    ).toBeInTheDocument();
  });

  it("omits the copy-link button when onShare is not provided", () => {
    render(<TracePanelHeader {...baseProps} />);
    expect(
      screen.queryByRole("button", { name: "Copy link to this trace" }),
    ).toBeNull();
  });

  it("always renders a copy-trace-id button next to the id", () => {
    render(<TracePanelHeader {...baseProps} />);
    expect(
      screen.getByRole("button", { name: "Copy trace ID" }),
    ).toBeInTheDocument();
  });
});
