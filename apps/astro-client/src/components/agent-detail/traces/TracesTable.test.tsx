import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup, fireEvent } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { TracesTable } from "./TracesTable";
import type { TraceEntry } from "@/lib/api";

afterEach(cleanup);

function makeTrace(over: Partial<TraceEntry>): TraceEntry {
  return {
    trace_id: "trace-0000",
    name: "chat completion",
    status: "success",
    latency_ms: 120,
    total_cost: 0,
    input: "",
    output: "",
    timestamp: "2026-07-07T00:00:00.000Z",
    ...over,
  };
}

// Trace IDs are short (<=16 chars) so they render verbatim in the table, which
// is what we assert on (the name is searchable but not a visible column).
const traces = [
  makeTrace({ trace_id: "id-chat-01", name: "chat completion" }),
  makeTrace({ trace_id: "id-tool-02", name: "tool call" }),
  makeTrace({ trace_id: "id-sum-03", name: "summarize thread", status: "error" }),
];

function searchBox() {
  return screen.getByRole("textbox", { name: /search traces/i });
}

describe("TracesTable search", () => {
  it("shows all trace rows before any search", () => {
    renderWithProviders(<TracesTable traces={traces} account="testuser" />);
    expect(screen.getByText("id-chat-01")).toBeInTheDocument();
    expect(screen.getByText("id-tool-02")).toBeInTheDocument();
    expect(screen.getByText("id-sum-03")).toBeInTheDocument();
  });

  it("filters by the span name even though it is not a visible column", () => {
    renderWithProviders(<TracesTable traces={traces} account="testuser" />);
    fireEvent.change(searchBox(), { target: { value: "tool" } });

    expect(screen.getByText("id-tool-02")).toBeInTheDocument();
    expect(screen.queryByText("id-chat-01")).toBeNull();
    expect(screen.queryByText("id-sum-03")).toBeNull();
    expect(screen.getByText("1 trace")).toBeInTheDocument();
  });

  it("filters by trace ID", () => {
    renderWithProviders(<TracesTable traces={traces} account="testuser" />);
    fireEvent.change(searchBox(), { target: { value: "id-sum" } });

    expect(screen.getByText("id-sum-03")).toBeInTheDocument();
    expect(screen.queryByText("id-tool-02")).toBeNull();
  });

  it("shows the empty state when nothing matches", () => {
    renderWithProviders(<TracesTable traces={traces} account="testuser" />);
    fireEvent.change(searchBox(), { target: { value: "no-such-trace" } });

    expect(screen.getByText(/no traces found/i)).toBeInTheDocument();
  });

  it("filters by the displayed user name, not just the raw user id", () => {
    const withUser = [
      makeTrace({ trace_id: "id-ada-1", user_id: "u-9f2c", user_details: { kind: "astro", display_name: "Ada Lovelace", username: "ada" } }),
      makeTrace({ trace_id: "id-bob-2", user_id: "u-11ab", user_details: { kind: "astro", display_name: "Bob Stone", username: "bob" } }),
    ];
    renderWithProviders(<TracesTable traces={withUser} account="testuser" />);
    fireEvent.change(searchBox(), { target: { value: "lovelace" } });

    expect(screen.getByText("id-ada-1")).toBeInTheDocument();
    expect(screen.queryByText("id-bob-2")).toBeNull();
  });
});
