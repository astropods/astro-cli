import { describe, it, expect, vi, afterEach } from "vitest";
import { screen, cleanup, fireEvent } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { LogViewer } from "./LogViewer";
import type { LogEntry } from "@/lib/log-utils";

afterEach(cleanup);

const LOGS: LogEntry[] = [
  { timestamp: "2024-01-01T00:00:01.000Z", level: "INFO",  message: "Agent initialized and ready" },
  { timestamp: "2024-01-01T00:00:02.000Z", level: "INFO",  message: "Received request id=req-001" },
  { timestamp: "2024-01-01T00:00:03.000Z", level: "ERROR", message: "failed to connect to database" },
  { timestamp: "2024-01-01T00:00:04.000Z", level: "WARN",  message: "retry attempt 1" },
  { timestamp: "2024-01-01T00:00:05.000Z", level: "INFO",  message: "Response generated in 200ms" },
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

describe("LogViewer", () => {
  it("renders all log lines", () => {
    renderViewer();
    expect(screen.getByText(/Agent initialized and ready/)).toBeInTheDocument();
    expect(screen.getByText(/Received request/)).toBeInTheDocument();
    expect(screen.getByText(/Response generated/)).toBeInTheDocument();
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

  it("displays normalised level labels", () => {
    renderViewer();
    expect(screen.getAllByText("INFO").length).toBeGreaterThan(0);
    expect(screen.getByText("ERROR")).toBeInTheDocument();
    expect(screen.getByText("WARN")).toBeInTheDocument();
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

  it("calls onTimeRangeChange when time range is changed", () => {
    const onTimeRangeChange = vi.fn();
    renderViewer({ onTimeRangeChange });
    fireEvent.click(screen.getByText("Last 15 min"));
    fireEvent.click(screen.getByText("Last 1 hour"));
    expect(onTimeRangeChange).toHaveBeenCalledWith("1h");
  });

  it("does not render live button without onLiveToggle", () => {
    renderViewer();
    expect(screen.queryByRole("button", { name: /Live/i })).not.toBeInTheDocument();
  });

  it("renders live button when onLiveToggle is provided", () => {
    renderViewer({ onLiveToggle: vi.fn() });
    expect(screen.getByRole("button", { name: /Live/i })).toBeInTheDocument();
  });

  it("calls onLiveToggle when live button is clicked", () => {
    const onLiveToggle = vi.fn();
    renderViewer({ onLiveToggle });
    fireEvent.click(screen.getByRole("button", { name: /Live/i }));
    expect(onLiveToggle).toHaveBeenCalledOnce();
  });

  it("shows pulsing indicator when live is active", () => {
    renderViewer({ onLiveToggle: vi.fn(), isLive: true });
    const dot = screen.getByRole("button", { name: /Live/i }).querySelector("span");
    expect(dot?.className).toContain("animate-pulse");
  });

  it("calls onLiveToggle when time range selector is opened while live", () => {
    const onLiveToggle = vi.fn();
    renderViewer({ onLiveToggle, isLive: true });
    fireEvent.click(screen.getByText("Last 15 min"));
    expect(onLiveToggle).toHaveBeenCalledOnce();
  });

  it("does not call onLiveToggle when time range is opened while not live", () => {
    const onLiveToggle = vi.fn();
    renderViewer({ onLiveToggle, isLive: false });
    fireEvent.click(screen.getByText("Last 15 min"));
    expect(onLiveToggle).not.toHaveBeenCalled();
  });

  it("renders error message when error prop is set", () => {
    renderViewer({ logs: [], error: "Failed to load logs." });
    expect(screen.getByText("Failed to load logs.")).toBeInTheDocument();
  });
});
