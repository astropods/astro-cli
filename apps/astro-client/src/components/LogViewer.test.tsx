import { describe, it, expect, vi, afterEach } from "vitest";
import { screen, cleanup, fireEvent } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { LogViewer } from "./LogViewer";

afterEach(cleanup);

const LOGS = [
  "2024-01-01T00:00:01.000Z Agent initialized and ready",
  "2024-01-01T00:00:02.000Z Received request id=req-001",
  "2024-01-01T00:00:03.000Z Error: failed to connect to database",
  "2024-01-01T00:00:04.000Z Warning: retry attempt 1",
  "2024-01-01T00:00:05.000Z Response generated in 200ms",
];

function renderViewer(props: Partial<React.ComponentProps<typeof LogViewer>> = {}) {
  return renderWithProviders(
    <LogViewer
      logs={LOGS}
      timeRange="15m"
      onTimeRangeChange={vi.fn()}
      {...props}
    />,
  );
}

// ── rendering ─────────────────────────────────────────────────────────────────

describe("LogViewer", () => {
  it("renders all log lines", () => {
    renderViewer();
    expect(screen.getByText(/Agent initialized and ready/)).toBeInTheDocument();
    expect(screen.getByText(/Received request/)).toBeInTheDocument();
    expect(screen.getByText(/Response generated/)).toBeInTheDocument();
  });

  it("shows line numbers starting at 1", () => {
    renderViewer();
    const lineNumbers = screen.getAllByText(/^\d+$/).map((el) => el.textContent);
    expect(lineNumbers).toContain("1");
    expect(lineNumbers).toContain(String(LOGS.length));
  });

  it("shows empty state when no logs", () => {
    renderViewer({ logs: [] });
    expect(screen.getByText("No log lines in this time window")).toBeInTheDocument();
  });

  it("shows loading spinner when isLoading", () => {
    renderViewer({ logs: [], isLoading: true });
    expect(screen.getByText("Loading logs…")).toBeInTheDocument();
  });

  it("renders leading slot content", () => {
    renderViewer({ leading: <button>Custom action</button> });
    expect(screen.getByRole("button", { name: "Custom action" })).toBeInTheDocument();
  });

  it("renders time range select with current value", () => {
    renderViewer({ timeRange: "1h" });
    expect(screen.getByText("Last 1 hour")).toBeInTheDocument();
  });

  // ── error filter ────────────────────────────────────────────────────────────

  it("counts error lines in the Errors button", () => {
    renderViewer();
    const errBtn = screen.getByRole("button", { name: /Errors/i });
    expect(errBtn).toHaveTextContent("1");
  });

  it("filters to only error lines when Errors is toggled", () => {
    renderViewer();
    fireEvent.click(screen.getByRole("button", { name: /Errors/i }));
    expect(screen.getByText(/failed to connect/)).toBeInTheDocument();
    expect(screen.queryByText(/Agent initialized/)).not.toBeInTheDocument();
  });

  it("clears error filter when Errors is toggled again", () => {
    renderViewer();
    const errBtn = screen.getByRole("button", { name: /Errors/i });
    fireEvent.click(errBtn);
    fireEvent.click(errBtn);
    expect(screen.getByText(/Agent initialized/)).toBeInTheDocument();
  });

  // ── warning filter ──────────────────────────────────────────────────────────

  it("counts warning lines in the Warnings button", () => {
    renderViewer();
    const warnBtn = screen.getByRole("button", { name: /Warnings/i });
    expect(warnBtn).toHaveTextContent("1");
  });

  it("filters to only warning lines when Warnings is toggled", () => {
    renderViewer();
    fireEvent.click(screen.getByRole("button", { name: /Warnings/i }));
    expect(screen.getByText(/retry attempt/)).toBeInTheDocument();
    expect(screen.queryByText(/Agent initialized/)).not.toBeInTheDocument();
  });

  // ── search ──────────────────────────────────────────────────────────────────

  it("filters lines by search term", () => {
    renderViewer();
    const input = screen.getByPlaceholderText("Search logs");
    fireEvent.change(input, { target: { value: "req-001" } });
    expect(screen.getByText(/req-001/)).toBeInTheDocument();
    expect(screen.queryByText(/Agent initialized/)).not.toBeInTheDocument();
  });

  it("search is case-insensitive", () => {
    renderViewer();
    fireEvent.change(screen.getByPlaceholderText("Search logs"), { target: { value: "AGENT" } });
    expect(screen.getByText(/Agent initialized/)).toBeInTheDocument();
  });

  it("shows no matching lines message when search has no results", () => {
    renderViewer();
    fireEvent.change(screen.getByPlaceholderText("Search logs"), { target: { value: "xyzzy-nomatch" } });
    expect(screen.getByText("No matching lines")).toBeInTheDocument();
  });

  // ── time range ──────────────────────────────────────────────────────────────

  it("calls onTimeRangeChange when time range is changed", () => {
    const onTimeRangeChange = vi.fn();
    renderViewer({ onTimeRangeChange });
    // The select trigger shows the current label; open it and pick another option
    fireEvent.click(screen.getByText("Last 15 min"));
    fireEvent.click(screen.getByText("Last 1 hour"));
    expect(onTimeRangeChange).toHaveBeenCalledWith("1h");
  });
});
