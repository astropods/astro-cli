import { describe, it, expect, vi, afterEach } from "vitest";
import { screen, cleanup, fireEvent } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { ActionPanel, ErrorPanel, WarningPanel, SuccessPanel, NeutralPanel } from "./status-panel";

afterEach(cleanup);

// ── ActionPanel ───────────────────────────────────────────────────────────────

describe("ActionPanel", () => {
  it("renders title and primary button", () => {
    renderWithProviders(
      <ActionPanel title="Do the thing" primaryLabel="Go" onPrimary={vi.fn()} />,
    );
    expect(screen.getByText("Do the thing")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Go" })).toBeInTheDocument();
  });

  it("calls onPrimary directly when no confirmTitle", () => {
    const onPrimary = vi.fn();
    renderWithProviders(
      <ActionPanel title="t" primaryLabel="Go" onPrimary={onPrimary} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Go" }));
    expect(onPrimary).toHaveBeenCalledOnce();
  });

  it("renders and calls an optional secondary action", () => {
    const onPrimary = vi.fn();
    const onSecondary = vi.fn();
    renderWithProviders(
      <ActionPanel
        title="t"
        primaryLabel="Roll back"
        onPrimary={onPrimary}
        secondaryLabel="Pause"
        onSecondary={onSecondary}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Pause" }));
    expect(onSecondary).toHaveBeenCalledOnce();
    expect(onPrimary).not.toHaveBeenCalled();
  });

  it("opens confirmation dialog instead of calling onPrimary immediately", () => {
    const onPrimary = vi.fn();
    renderWithProviders(
      <ActionPanel
        title="t"
        primaryLabel="Redeploy"
        onPrimary={onPrimary}
        confirmTitle="Are you sure?"
        confirmBody="This could be destructive."
        confirmLabel="Confirm redeploy"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Redeploy" }));
    expect(onPrimary).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("Are you sure?")).toBeInTheDocument();
    expect(screen.getByText("This could be destructive.")).toBeInTheDocument();
  });

  it("calls onPrimary after confirming in dialog", () => {
    const onPrimary = vi.fn();
    renderWithProviders(
      <ActionPanel
        title="t"
        primaryLabel="Redeploy"
        onPrimary={onPrimary}
        confirmTitle="Sure?"
        confirmLabel="Confirm redeploy"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Redeploy" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm redeploy" }));
    expect(onPrimary).toHaveBeenCalledOnce();
  });

  it("closes dialog without calling onPrimary when cancelled", () => {
    const onPrimary = vi.fn();
    renderWithProviders(
      <ActionPanel
        title="t"
        primaryLabel="Redeploy"
        onPrimary={onPrimary}
        confirmTitle="Sure?"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Redeploy" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onPrimary).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("dismisses when X is clicked", () => {
    renderWithProviders(
      <ActionPanel title="Dismissable" primaryLabel="Go" onPrimary={vi.fn()} dismissible />,
    );
    expect(screen.getByText("Dismissable")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText("Dismissable")).not.toBeInTheDocument();
  });

  it("calls onDismiss callback when dismissed", () => {
    const onDismiss = vi.fn();
    renderWithProviders(
      <ActionPanel title="t" primaryLabel="Go" onPrimary={vi.fn()} dismissible onDismiss={onDismiss} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it("does not render dismiss button when dismissible is false", () => {
    renderWithProviders(
      <ActionPanel title="t" primaryLabel="Go" onPrimary={vi.fn()} />,
    );
    expect(screen.queryByRole("button", { name: "Dismiss" })).not.toBeInTheDocument();
  });
});

// ── Tone panels ───────────────────────────────────────────────────────────────

describe("ErrorPanel", () => {
  it("renders children", () => {
    renderWithProviders(<ErrorPanel>Something went wrong</ErrorPanel>);
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("renders title when provided", () => {
    renderWithProviders(<ErrorPanel title="Oops">detail</ErrorPanel>);
    expect(screen.getByText("Oops")).toBeInTheDocument();
  });

  it("dismisses when dismissible", () => {
    renderWithProviders(<ErrorPanel dismissible>Error text</ErrorPanel>);
    fireEvent.click(screen.getByRole("button", { name: "Dismiss panel" }));
    expect(screen.queryByText("Error text")).not.toBeInTheDocument();
  });
});

describe("WarningPanel", () => {
  it("renders children", () => {
    renderWithProviders(<WarningPanel>Watch out</WarningPanel>);
    expect(screen.getByText("Watch out")).toBeInTheDocument();
  });
});

describe("SuccessPanel", () => {
  it("renders children", () => {
    renderWithProviders(<SuccessPanel>All good</SuccessPanel>);
    expect(screen.getByText("All good")).toBeInTheDocument();
  });
});

describe("NeutralPanel", () => {
  it("renders children", () => {
    renderWithProviders(<NeutralPanel>Info here</NeutralPanel>);
    expect(screen.getByText("Info here")).toBeInTheDocument();
  });
});
