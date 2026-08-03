import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { SidePanel } from "./side-panel";

afterEach(cleanup);

describe("SidePanel", () => {
  it("renders the title, body, and a dialog with the aria label", () => {
    render(
      <SidePanel title={<span>Trace details</span>} onClose={() => {}} ariaLabel="Trace details">
        <p>body content</p>
      </SidePanel>,
    );
    const dialog = screen.getByRole("dialog", { name: "Trace details" });
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText("body content")).toBeInTheDocument();
  });

  it("fires onClose from the close button", () => {
    const onClose = vi.fn();
    render(
      <SidePanel title="t" onClose={onClose} closeLabel="Close trace">
        <div />
      </SidePanel>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Close trace" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("shows the expand control only when onToggleExpanded is provided and reflects expanded", () => {
    const onToggleExpanded = vi.fn();
    const { rerender } = render(
      <SidePanel title="t" onClose={() => {}}>
        <div />
      </SidePanel>,
    );
    expect(screen.queryByRole("button", { name: /expand panel|restore panel/i })).toBeNull();

    rerender(
      <SidePanel title="t" onClose={() => {}} onToggleExpanded={onToggleExpanded} expanded={false}>
        <div />
      </SidePanel>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Expand panel to full width" }));
    expect(onToggleExpanded).toHaveBeenCalledTimes(1);

    rerender(
      <SidePanel title="t" onClose={() => {}} onToggleExpanded={onToggleExpanded} expanded>
        <div />
      </SidePanel>,
    );
    expect(screen.getByRole("button", { name: "Restore panel size" })).toBeInTheDocument();
  });

  it("renders panel-specific header actions", () => {
    render(
      <SidePanel title="t" onClose={() => {}} headerActions={<button>nav</button>}>
        <div />
      </SidePanel>,
    );
    expect(screen.getByRole("button", { name: "nav" })).toBeInTheDocument();
  });

  it("drops the header divider when headerBorder is false", () => {
    const { rerender } = render(
      <SidePanel title="t" onClose={() => {}} ariaLabel="p">
        <div />
      </SidePanel>,
    );
    const headerOf = () =>
      screen.getByRole("dialog", { name: "p" }).firstElementChild as HTMLElement;
    expect(headerOf().className).toContain("border-b");

    rerender(
      <SidePanel title="t" onClose={() => {}} ariaLabel="p" headerBorder={false}>
        <div />
      </SidePanel>,
    );
    expect(headerOf().className).not.toContain("border-b");
  });

  it("does not render docked content while closed, and shows it once open", () => {
    const { rerender } = render(
      <SidePanel open={false} ariaLabel="Agent details" onClose={() => {}}>
        <p>docked body</p>
      </SidePanel>,
    );
    // Closed: nothing mounted (keeps the content's queries idle) and no inline dialog.
    expect(screen.queryByText("docked body")).toBeNull();
    expect(screen.queryByRole("dialog")).toBeNull();

    rerender(
      <SidePanel open ariaLabel="Agent details" onClose={() => {}}>
        <p>docked body</p>
      </SidePanel>,
    );
    expect(screen.getByText("docked body")).toBeInTheDocument();
    // Docked content brings its own header, so the inline title row is absent.
    expect(screen.queryByRole("button", { name: /close panel/i })).toBeNull();
  });
});
