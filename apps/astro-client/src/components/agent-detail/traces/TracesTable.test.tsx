import { useState } from "react";
import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup, fireEvent } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import {
  TracesTable,
  traceUserFilterParams,
  type TraceSortState,
} from "./TracesTable";
import type { TraceEntry } from "@/lib/api";

afterEach(cleanup);

function makeTrace(over: Partial<TraceEntry>): TraceEntry {
  return {
    trace_id: "trace-0000",
    name: "chat completion",
    status: "success",
    latency_ms: 120,
    total_cost: 0,
    timestamp: "2026-07-07T00:00:00.000Z",
    ...over,
  };
}

// Trace IDs are short (<=16 chars) so they render verbatim in the table, which
// is what we assert on (the name is searchable but not a visible column).
const traces = [
  makeTrace({ trace_id: "id-chat-01", name: "chat completion" }),
  makeTrace({ trace_id: "id-tool-02", name: "tool call" }),
  makeTrace({ trace_id: "id-sum-03", name: "summarize thread" }),
];

function searchBox() {
  return screen.getByRole("textbox", { name: /search traces/i });
}

function TestTracesTable({
  traces,
}: {
  traces: TraceEntry[];
}) {
  const [search, setSearch] = useState("");
  const [selectedUserKey, setSelectedUserKey] = useState<string | null>(null);
  const [sort, setSort] = useState<TraceSortState>({
    key: "timestamp",
    direction: "desc",
  });

  return (
    <TracesTable
      traces={traces}
      userFacets={traces.map((trace) => ({
        user_id: trace.user_id,
        user_details: trace.user_details,
        count: 1,
      }))}
      account="testuser"
      search={search}
      onSearchChange={setSearch}
      selectedUserKey={selectedUserKey}
      onSelectedUserKeyChange={setSelectedUserKey}
      sort={sort}
      onSortChange={setSort}
    />
  );
}

describe("TracesTable search", () => {
  it("forwards search state without filtering server-provided rows", () => {
    renderWithProviders(<TestTracesTable traces={traces} />);
    fireEvent.change(searchBox(), { target: { value: "tool" } });

    expect(searchBox()).toHaveValue("tool");
    expect(screen.getByText("id-chat-01")).toBeInTheDocument();
    expect(screen.getByText("id-tool-02")).toBeInTheDocument();
    expect(screen.getByText("id-sum-03")).toBeInTheDocument();
    expect(screen.getByText("3 traces")).toBeInTheDocument();
  });
});

describe("trace user API params", () => {
  it("translates UI selection keys to structured query params", () => {
    expect(traceUserFilterParams("user:user-ada")).toEqual({ user_id: "user-ada" });
    expect(traceUserFilterParams("__no_user__")).toEqual({ no_user: "true" });
    expect(traceUserFilterParams(null)).toEqual({});
  });
});

describe("TracesTable column controls", () => {
  const controlledTraces = [
    makeTrace({
      trace_id: "trace-ada",
      timestamp: "2026-07-07T03:00:00.000Z",
      latency_ms: 900,
      total_cost: 0.03,
      user_id: "user-ada",
      user_details: { kind: "astro", display_name: "Ada Lovelace", username: "ada" },
    }),
    makeTrace({
      trace_id: "trace-bob",
      timestamp: "2026-07-07T01:00:00.000Z",
      latency_ms: 100,
      total_cost: 0.01,
      user_id: "user-bob",
      user_details: { kind: "astro", display_name: "Bob Stone", username: "bob" },
    }),
  ];

  function renderedIds() {
    return screen.getAllByRole("row").slice(1).map((row) =>
      controlledTraces.find((trace) => row.textContent?.includes(trace.trace_id))?.trace_id,
    );
  }

  it("renders only meaningful trace columns", () => {
    renderWithProviders(<TestTracesTable traces={controlledTraces} />);

    expect(
      screen.getAllByRole("columnheader").map((header) => header.textContent?.trim()),
    ).toEqual(["Date↓", "User", "Latency", "Cost", "Trace ID"]);
  });

  it("updates the controlled user selection without filtering rows locally", () => {
    renderWithProviders(<TestTracesTable traces={controlledTraces} />);

    fireEvent.click(screen.getByRole("button", { name: /filter by user/i }));
    expect(document.querySelector('[data-slot="popover-content"]')).toHaveClass(
      "dark:bg-popover",
      "dark:text-popover-foreground",
    );
    expect(screen.getByText("Users and counts reflect the selected window.")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("Filter users"), {
      target: { value: "Ada" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Ada Lovelace/i }));

    expect(screen.getByText("trace-ada")).toBeInTheDocument();
    expect(screen.getByText("trace-bob")).toBeInTheDocument();
  });

  it("searches server-resolved Astro profiles", async () => {
    const astroTraces = [
      makeTrace({
        trace_id: "trace-jeseye",
        user_id: "user_jeseye",
        user_details: { kind: "astro", display_name: "Jeseye", username: "jeseye" },
      }),
      makeTrace({
        trace_id: "trace-sohum",
        user_id: "user_sohum",
        user_details: { kind: "astro", display_name: "Sohum", username: "sohum" },
      }),
    ];
    renderWithProviders(<TestTracesTable traces={astroTraces} />);

    fireEvent.click(screen.getByRole("button", { name: /filter by user/i }));
    fireEvent.change(screen.getByPlaceholderText("Filter users"), {
      target: { value: "Sohum" },
    });

    expect(await screen.findAllByText("Sohum")).not.toHaveLength(0);
    expect(screen.queryByRole("button", { name: /Jeseye/i })).not.toBeInTheDocument();
    expect(screen.queryByText("user_sohum")).not.toBeInTheDocument();
  });

  it("renders avatar-only identities through the shared avatar component", () => {
    renderWithProviders(
      <TestTracesTable
        traces={[
          makeTrace({
            trace_id: "trace-slack-ada",
            user_id: "U-ADA",
            user_details: {
              kind: "slack",
              display_name: "Slack Ada",
              avatar_url: "https://example.com/ada.png",
            },
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /filter by user/i }));

    expect(screen.getByRole("img", { name: "Slack Ada" })).toHaveAttribute(
      "src",
      "https://example.com/ada.png",
    );
  });

  it("updates sort controls without reordering server-provided rows", () => {
    renderWithProviders(<TestTracesTable traces={controlledTraces} />);

    expect(renderedIds()).toEqual(["trace-ada", "trace-bob"]);

    fireEvent.click(screen.getByRole("button", { name: /sort by latency/i }));
    expect(renderedIds()).toEqual(["trace-ada", "trace-bob"]);
    expect(screen.getByText("Latency").closest("th")).toHaveAttribute("aria-sort", "descending");
    fireEvent.click(screen.getByRole("button", { name: /sort by latency/i }));
    expect(renderedIds()).toEqual(["trace-ada", "trace-bob"]);
    expect(screen.getByText("Latency").closest("th")).toHaveAttribute("aria-sort", "ascending");
  });
});
