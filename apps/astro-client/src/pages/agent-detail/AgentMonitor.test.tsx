import { describe, it, expect, afterEach, vi } from "vitest";
import { screen, cleanup, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { Outlet, useLocation } from "react-router";
import { server } from "@/test/msw/server";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import { traceRowAnchorId } from "@/lib/routes";
import type {
  AgentDeployment,
  MetricsBucket,
  TraceEntry,
  ObservabilityMetricsResponse,
  ObservabilityTracesResponse,
} from "@/lib/api";
import AgentMonitor from "./AgentMonitor";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock("@/hooks/use-container-size", () => ({
  useContainerSize: () => ({ ref: { current: null }, width: 1200, height: 800 }),
}));

afterEach(cleanup);
afterEach(() => server.resetHandlers());

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// Use a recent timestamp so buckets fall within the aggregation window
const RECENT = new Date(Date.now() - 1000 * 60 * 60 * 12).toISOString(); // 12h ago

function makeBucket(overrides?: Partial<MetricsBucket>): MetricsBucket {
  return {
    timestamp: RECENT,
    trace_count: 5,
    avg_latency_ms: 250,
    p95_latency_ms: 500,
    min_latency_ms: 100,
    max_latency_ms: 600,
    input_tokens: 1000,
    output_tokens: 800,
    error_count: 0,
    ...overrides,
  };
}

function makeTrace(overrides?: Partial<TraceEntry>): TraceEntry {
  return {
    trace_id: "trace-001",
    name: "chat",
    status: "success",
    latency_ms: 245,
    total_cost: 0.0012,
    input: "Hello, summarize this document.",
    output: "Here is the summary of the document.",
    timestamp: RECENT,
    ...overrides,
  };
}

function makeDeployment(overrides?: Partial<AgentDeployment>): AgentDeployment {
  return {
    id: "dep-1",
    name: "my-agent",
    build_id: "a1b2c3d4",
    namespace: "astro-ns",
    status: "Running",
    replicas: 1,
    created_at: "2025-04-01T00:00:00Z",
    components: ["agent"],
    avatar_colors: { accent: "#2dd4bf", base: "#0f766e", vibrant: "#2dd4bf", vibrant_light: "#5eead4", accent_light: "#99f6e4", background: "#042f2e", foreground: "#f0fdfa", glow: "#2dd4bf" },
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// MSW helpers
// ---------------------------------------------------------------------------

const emptyMetrics: ObservabilityMetricsResponse = {
  buckets: [],
  time_range: { start: "2025-04-21T00:00:00Z", end: "2025-04-28T00:00:00Z" },
  interval_minutes: 60,
};

const emptyTraces: ObservabilityTracesResponse = {
  traces: [],
  total: 0,
  limit: 100,
  offset: 0,
};

const emptyRect: DOMRect = {
  top: 0,
  bottom: 0,
  left: 0,
  right: 0,
  width: 0,
  height: 0,
  x: 0,
  y: 0,
  toJSON: () => ({}),
};

function setupHandlers(
  metrics: ObservabilityMetricsResponse = emptyMetrics,
  traces: ObservabilityTracesResponse = emptyTraces,
) {
  server.use(
    http.get("/api/v1/deployments/:id/observability/metrics", () =>
      HttpResponse.json(metrics),
    ),
    http.get("/api/v1/deployments/:id/observability/traces", () =>
      HttpResponse.json(traces),
    ),
  );
}

// ---------------------------------------------------------------------------
// Rendering helper
// ---------------------------------------------------------------------------

function renderMonitor(deployment?: AgentDeployment, initialEntry?: string) {
  const dep = deployment ?? makeDeployment();
  const user = userEvent.setup();
  let currentLocation = "";

  function AgentDetailTestShell() {
    const location = useLocation();
    currentLocation = `${location.pathname}${location.search}`;
    return (
      <Outlet
        context={{
          deployment: dep,
          account: "testuser",
          deploymentId: dep.id,
        }}
      />
    );
  }

  const result = renderRoute(
    [
      {
        path: "/:account/agents/:deploymentId",
        Component: AgentDetailTestShell,
        children: [
          { path: "monitor", Component: AgentMonitor },
        ],
      },
    ],
    {
      initialEntries: [initialEntry ?? `/testuser/agents/${dep.id}/monitor`],
      auth: mockAuthContext,
    },
  );

  return { ...result, user, getLocation: () => currentLocation };
}

// ===========================================================================
// Tests
// ===========================================================================

describe("user views token usage", () => {
  it("shows heading and input/output totals when data exists", async () => {
    setupHandlers({
      ...emptyMetrics,
      buckets: [makeBucket({ input_tokens: 1200, output_tokens: 850 })],
    });
    renderMonitor();
    expect(await screen.findByText("Token Usage")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText(/1\.2K input/)).toBeInTheDocument();
    });
    expect(screen.getByText(/850 output/)).toBeInTheDocument();
  });

  it("shows zeroed chart when metrics are empty", async () => {
    setupHandlers();
    renderMonitor();
    // Chart legend renders even with no data — the chart shows with zero values
    expect(await screen.findByText("Input tokens")).toBeInTheDocument();
    expect(screen.getByText("Output tokens")).toBeInTheDocument();
    // Subtitle still shows with zeroed totals
    expect(screen.getByText(/0 input/)).toBeInTheDocument();
  });
});

describe("user views requests and latency", () => {
  it("shows heading and total request count", async () => {
    setupHandlers({
      ...emptyMetrics,
      buckets: [makeBucket({ trace_count: 42, input_tokens: 100, output_tokens: 50 })],
    });
    renderMonitor();
    expect(await screen.findByText("Requests & Latency")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText(/42 total requests/)).toBeInTheDocument();
    });
  });

  it("shows the user's avg latency and p95 values", async () => {
    setupHandlers({
      ...emptyMetrics,
      buckets: [makeBucket({ trace_count: 10, avg_latency_ms: 250, p95_latency_ms: 500 })],
    });
    renderMonitor();

    // Avg Latency hero metric — under 1s the source formats as "{ms}ms"
    const avgLabel = await screen.findByText("Avg Latency");
    const avgCard = avgLabel.parentElement!;
    expect(within(avgCard).getByText("250ms")).toBeInTheDocument();

    // P95 secondary metric
    const p95Label = screen.getByText("P95");
    const p95Card = p95Label.parentElement!;
    expect(within(p95Card).getByText("500ms")).toBeInTheDocument();
  });

  it("shows zeroed charts and latency when no requests exist", async () => {
    setupHandlers();
    renderMonitor();
    // Subtitle renders with zeroed totals once the chart settles
    await waitFor(() => {
      expect(screen.getByText(/0 total requests/)).toBeInTheDocument();
    });
    // Latency card shows the empty state when there are no requests
    expect(screen.getByText("Avg Latency")).toBeInTheDocument();
    expect(screen.getByText("No requests in this range")).toBeInTheDocument();
  });
});

describe("user switches time range", () => {
  it("clicking 14D shows totals for the wider 14-day window", async () => {
    // Return different totals depending on how many days back start_time is —
    // proves the user actually sees 14-day data after clicking, not just that
    // a fetch happened.
    server.use(
      http.get("/api/v1/deployments/:id/observability/metrics", ({ request }) => {
        const url = new URL(request.url);
        const start = new Date(url.searchParams.get("start_time")!);
        const days = Math.round((Date.now() - start.getTime()) / 86_400_000);
        const isWide = days >= 10;
        return HttpResponse.json<ObservabilityMetricsResponse>({
          buckets: [
            makeBucket({
              input_tokens: isWide ? 3000 : 1000,
              output_tokens: isWide ? 2000 : 500,
            }),
          ],
          time_range: { start: start.toISOString(), end: new Date().toISOString() },
          interval_minutes: 60,
        });
      }),
      http.get("/api/v1/deployments/:id/observability/traces", () =>
        HttpResponse.json(emptyTraces),
      ),
    );
    const { user } = renderMonitor();

    // Initial 7D view — narrow window totals
    expect(await screen.findByText(/1\.0K input · 500 output/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "14D" }));

    // After switching, wider window totals appear
    expect(await screen.findByText(/3\.0K input · 2\.0K output/)).toBeInTheDocument();
  });

  it("marks the clicked range as the active selection", async () => {
    setupHandlers();
    const { user } = renderMonitor();
    await screen.findByText("Token Usage");

    expect(screen.getByRole("button", { name: "7D" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "14D" })).toHaveAttribute("aria-pressed", "false");

    await user.click(screen.getByRole("button", { name: "14D" }));

    expect(screen.getByRole("button", { name: "14D" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "7D" })).toHaveAttribute("aria-pressed", "false");
  });
});

describe("user views traces", () => {
  it("shows traces table with status, latency, cost, and trace ID", async () => {
    setupHandlers(emptyMetrics, {
      traces: [
        makeTrace({ trace_id: "abc-123", status: "success", latency_ms: 245, total_cost: 0.0012 }),
        makeTrace({ trace_id: "def-456", status: "error", latency_ms: 1500, total_cost: 0.05 }),
      ],
      total: 2,
      limit: 100,
      offset: 0,
    });
    renderMonitor();
    // Wait for trace data to arrive (trace ID is data-specific, not a static label)
    expect(await screen.findByText("abc-123")).toBeInTheDocument();
    expect(screen.getByText("def-456")).toBeInTheDocument();
    expect(screen.getByText("245ms")).toBeInTheDocument();
    expect(screen.getByText("1.5s")).toBeInTheDocument();
    expect(screen.getByText("2 traces")).toBeInTheDocument();
  });

  it("shows no-traces message when response is empty", async () => {
    setupHandlers();
    renderMonitor();
    expect(await screen.findByText("No traces found.")).toBeInTheDocument();
  });
});

describe("user filters traces by status", () => {
  function setupFilterTraces() {
    setupHandlers(emptyMetrics, {
      traces: [
        makeTrace({ trace_id: "t-ok-1", status: "success", latency_ms: 100 }),
        makeTrace({ trace_id: "t-ok-2", status: "success", latency_ms: 200 }),
        makeTrace({ trace_id: "t-err", status: "error", latency_ms: 300 }),
        makeTrace({ trace_id: "t-timeout", status: "timeout", latency_ms: 5000 }),
      ],
      total: 4,
      limit: 100,
      offset: 0,
    });
  }

  it("selecting Error from the dropdown shows only error traces", async () => {
    setupFilterTraces();
    const { user } = renderMonitor();
    expect(await screen.findByText("4 traces")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /all statuses/i }));
    await user.click(screen.getByRole("button", { name: /^error$/i }));

    await waitFor(() => {
      expect(screen.getByText("1 trace")).toBeInTheDocument();
    });
    expect(screen.getByText("t-err")).toBeInTheDocument();
    expect(screen.queryByText("t-ok-1")).not.toBeInTheDocument();
  });

  it("clicking All statuses returns to showing every trace", async () => {
    setupFilterTraces();
    const { user } = renderMonitor();
    await screen.findByText("4 traces");

    await user.click(screen.getByRole("button", { name: /all statuses/i }));
    await user.click(screen.getByRole("button", { name: /^error$/i }));
    await waitFor(() => expect(screen.getByText("1 trace")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /all statuses/i }));
    await waitFor(() => expect(screen.getByText("4 traces")).toBeInTheDocument());
  });
});

describe("user loads more traces", () => {
  it("shows a Load more button and appends the next page when clicked", async () => {
    const all = Array.from({ length: 15 }, (_, i) =>
      makeTrace({ trace_id: `trace-${String(i).padStart(3, "0")}` }),
    );
    // Paginate by the request offset. The response's limit/total drive
    // hasNextPage, so this exercises Load more independent of the client's
    // page size.
    server.use(
      http.get("/api/v1/deployments/:id/observability/metrics", () =>
        HttpResponse.json(emptyMetrics),
      ),
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        const offset = Number(
          new URL(request.url).searchParams.get("offset") ?? "0",
        );
        const pageSize = 10;
        return HttpResponse.json({
          traces: all.slice(offset, offset + pageSize),
          total: all.length,
          limit: pageSize,
          offset,
        });
      }),
    );
    const { user } = renderMonitor();
    expect(await screen.findByText("trace-000")).toBeInTheDocument();
    expect(screen.queryByText("trace-014")).not.toBeInTheDocument();

    await user.click(screen.getByText(/load more/i));
    expect(await screen.findByText("trace-014")).toBeInTheDocument();
    expect(screen.queryByText(/load more/i)).not.toBeInTheDocument();
  });
});

describe("user inspects trace details", () => {
  function setupDetailTraces() {
    setupHandlers(emptyMetrics, {
      traces: [
        makeTrace({
          trace_id: "t-1",
          status: "success",
          latency_ms: 245,
          total_cost: 0.0012,
          input: "What is the weather?",
          output: "It is sunny today.",
        }),
        makeTrace({
          trace_id: "t-2",
          status: "error",
          latency_ms: 1500,
          total_cost: 0.05,
          input: "Complex query",
          output: "",
        }),
      ],
      total: 2,
      limit: 100,
      offset: 0,
    });
  }

  it("clicking a trace row opens the detail panel with status, latency, and cost", async () => {
    setupDetailTraces();
    const { user, getLocation } = renderMonitor();
    await user.click((await screen.findByText("t-1")).closest("tr")!);

    // Scope to the panel — the table also has Status/Latency/Cost column headers.
    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("Success")).toBeInTheDocument();
    expect(within(panel).getByText("245ms")).toBeInTheDocument();
    expect(within(panel).getByText("$0.0012")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLocation()).toBe("/testuser/agents/dep-1/monitor?trace=t-1");
    });
  });

  it("opens the detail panel from the trace query parameter", async () => {
    setupDetailTraces();
    renderMonitor(undefined, "/testuser/agents/dep-1/monitor?trace=t-2");

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("t-2")).toBeInTheDocument();
    expect(within(panel).getByText("Complex query")).toBeInTheDocument();
  });

  it("expands and scrolls to the selected trace row from a trace anchor", async () => {
    const traces = Array.from({ length: 12 }, (_, i) =>
      makeTrace({ trace_id: `trace-${String(i).padStart(3, "0")}` }),
    );
    const targetTraceId = "trace-011";
    const targetRowId = traceRowAnchorId(targetTraceId);
    const scrollIntoView = vi.spyOn(
      window.HTMLElement.prototype,
      "scrollIntoView",
    );
    // Deep-linked row lands off-screen; jsdom otherwise reports a zero rect,
    // which the visibility guard would treat as already-visible.
    const originalRect = window.HTMLElement.prototype.getBoundingClientRect;
    const rectSpy = vi
      .spyOn(window.HTMLElement.prototype, "getBoundingClientRect")
      .mockImplementation(function (this: HTMLElement) {
        if (this.id === targetRowId) {
          return { ...emptyRect, top: 2000, bottom: 2040, height: 40 };
        }
        return originalRect.call(this);
      });
    setupHandlers(emptyMetrics, { traces, total: 12, limit: 100, offset: 0 });
    renderMonitor(
      undefined,
      `/testuser/agents/dep-1/monitor?trace=${targetTraceId}#${traceRowAnchorId(targetTraceId)}`,
    );

    await waitFor(() => {
      expect(document.getElementById(targetRowId)).toBeInTheDocument();
    });
    expect(document.getElementById(targetRowId)).toHaveAttribute(
      "data-selected",
      "true",
    );
    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledWith({
        block: "center",
        inline: "nearest",
      });
    });
    scrollIntoView.mockRestore();
    rectSpy.mockRestore();
  });

  it("does not scroll when the selected trace row is already fully visible", async () => {
    const traces = Array.from({ length: 3 }, (_, i) =>
      makeTrace({ trace_id: `t-${i}` }),
    );
    const scrollIntoView = vi.spyOn(
      window.HTMLElement.prototype,
      "scrollIntoView",
    );
    setupHandlers(emptyMetrics, { traces, total: 3, limit: 100, offset: 0 });
    const { user } = renderMonitor();

    await user.click((await screen.findByText("t-1")).closest("tr")!);

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("t-1")).toBeInTheDocument();
    // jsdom reports a zero rect (top: 0, bottom: 0), which is within the
    // viewport, so the visibility guard should skip the recenter.
    expect(scrollIntoView).not.toHaveBeenCalled();
    scrollIntoView.mockRestore();
  });

  it("shows Input and Output sections with trace content", async () => {
    setupDetailTraces();
    const { user } = renderMonitor();
    await user.click((await screen.findByText("t-1")).closest("tr")!);

    expect(await screen.findByText("Input")).toBeInTheDocument();
    expect(screen.getByText("Output")).toBeInTheDocument();
    expect(screen.getByText("What is the weather?")).toBeInTheDocument();
    expect(screen.getByText("It is sunny today.")).toBeInTheDocument();
  });

  it("shows 'No content.' when output is empty", async () => {
    setupDetailTraces();
    const { user } = renderMonitor();
    // Click the error trace with empty output
    await user.click((await screen.findByText("t-2")).closest("tr")!);
    expect(await screen.findByText("No content.")).toBeInTheDocument();
  });

  it("close button dismisses the panel", async () => {
    setupDetailTraces();
    const { user, getLocation } = renderMonitor(undefined, "/testuser/agents/dep-1/monitor?trace=t-1");
    expect(await screen.findByText("What is the weather?")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /close trace/i }));
    await waitFor(() => {
      expect(screen.queryByText("What is the weather?")).not.toBeInTheDocument();
    });
    expect(getLocation()).toBe("/testuser/agents/dep-1/monitor");
  });

  it("clears a selected trace when closing a clicked row", async () => {
    setupDetailTraces();
    const { user, getLocation } = renderMonitor();
    await user.click((await screen.findByText("t-1")).closest("tr")!);
    expect(await screen.findByText("What is the weather?")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /close trace/i }));
    await waitFor(() => {
      expect(screen.queryByText("What is the weather?")).not.toBeInTheDocument();
    });
    expect(getLocation()).toBe("/testuser/agents/dep-1/monitor");
  });

  it("pretty-prints JSON input as a code block", async () => {
    setupHandlers(emptyMetrics, {
      traces: [
        makeTrace({
          trace_id: "t-json",
          input: '{"user":"alice","action":"summarize"}',
          output: "OK",
        }),
      ],
      total: 1,
      limit: 100,
      offset: 0,
    });
    const { user } = renderMonitor();
    await user.click((await screen.findByText("t-json")).closest("tr")!);

    // The compact JSON should be reformatted with indentation: each key on its
    // own line preceded by two spaces. Match a substring that only exists if
    // pretty-printing happened.
    const inputSection = (await screen.findByText("Input")).closest("section")!;
    const code = inputSection.querySelector("code")!;
    expect(code.textContent).toContain('"user": "alice"');
    expect(code.textContent).toContain('"action": "summarize"');
  });
});

describe("user navigates between traces", () => {
  function setupNavTraces() {
    setupHandlers(emptyMetrics, {
      traces: [
        makeTrace({ trace_id: "first", input: "First question", output: "First answer" }),
        makeTrace({ trace_id: "second", input: "Second question", output: "Second answer" }),
        makeTrace({ trace_id: "third", input: "Third question", output: "Third answer" }),
      ],
      total: 3,
      limit: 100,
      offset: 0,
    });
  }

  it("Next button navigates to the next trace", async () => {
    setupNavTraces();
    const { user, getLocation } = renderMonitor();
    await user.click((await screen.findByText("first")).closest("tr")!);
    expect(await screen.findByText("First question")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLocation()).toBe("/testuser/agents/dep-1/monitor?trace=first");
    });

    await user.click(screen.getByRole("button", { name: /next trace/i }));
    expect(await screen.findByText("Second question")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLocation()).toBe("/testuser/agents/dep-1/monitor?trace=second");
    });
  });

  it("Prev is disabled on the first trace", async () => {
    setupNavTraces();
    const { user } = renderMonitor();
    await user.click((await screen.findByText("first")).closest("tr")!);
    await screen.findByText("First question");

    expect(screen.getByRole("button", { name: /previous trace/i })).toBeDisabled();
  });

  it("Next is disabled on the last trace", async () => {
    setupNavTraces();
    const { user } = renderMonitor();
    await user.click((await screen.findByText("third")).closest("tr")!);
    await screen.findByText("Third question");

    expect(screen.getByRole("button", { name: /next trace/i })).toBeDisabled();
  });
});
