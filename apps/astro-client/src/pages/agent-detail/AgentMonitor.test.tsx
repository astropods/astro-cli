import { describe, it, expect, afterEach, vi } from "vitest";
import { screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";
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
  TraceDetailResponse,
  TraceUserFacetsResponse,
} from "@/lib/api";
import AgentMonitor from "./AgentMonitor";
import AgentTraces from "./AgentTraces";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const containerSize = vi.hoisted(() => ({ width: 1200 }));

vi.mock("@/hooks/use-container-size", () => ({
  useContainerSize: () => ({
    ref: { current: null },
    width: containerSize.width,
    height: 800,
  }),
}));

afterEach(cleanup);
afterEach(() => {
  containerSize.width = 1200;
  traceFixtures.clear();
  server.resetHandlers();
  vi.unstubAllGlobals();
});

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

type TraceFixture = TraceEntry & { input?: unknown; output?: unknown };

const traceFixtures = new Map<string, TraceFixture>();

function makeTrace(overrides?: Partial<TraceFixture>): TraceFixture {
  const trace = {
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
  traceFixtures.set(trace.trace_id, trace);
  return trace;
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

function traceUserFacetsHandler(traces: TraceEntry[]) {
  const users = new Map<string, TraceUserFacetsResponse["users"][number]>();
  for (const trace of traces) {
    const key = trace.user_id ?? "";
    const existing = users.get(key);
    if (existing) existing.count += 1;
    else users.set(key, {
      user_id: trace.user_id,
      user_details: trace.user_details,
      count: 1,
    });
  }
  return http.get("/api/v1/deployments/:id/observability/trace-users", () =>
    HttpResponse.json<TraceUserFacetsResponse>({ users: [...users.values()] }),
  );
}

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
      HttpResponse.json({
        ...traces,
        traces: traces.traces.map((trace) => {
          const fixture = trace as TraceFixture;
          const { input, output, ...entry } = fixture;
          void input;
          void output;
          return entry;
        }),
      }),
    ),
    http.get("/api/v1/deployments/:id/observability/traces/:traceId", ({ params }) => {
      const traceId = String(params.traceId);
      const fixture = (traces.traces as TraceFixture[]).find(
        (trace) => trace.trace_id === traceId,
      ) ?? traceFixtures.get(traceId);
      if (!fixture) return HttpResponse.json({ error: "not found" }, { status: 404 });
      return HttpResponse.json<TraceDetailResponse>({
        trace: {
          trace_id: fixture.trace_id,
          name: fixture.name,
          timestamp: fixture.timestamp,
          latency_ms: fixture.latency_ms,
          total_cost: fixture.total_cost ?? 0,
          input: fixture.input,
          output: fixture.output,
          user_id: fixture.user_id,
          user_details: fixture.user_details,
        },
        observations: [],
        scores: [],
      });
    }),
    traceUserFacetsHandler(traces.traces),
  );
}

// ---------------------------------------------------------------------------
// Rendering helper
// ---------------------------------------------------------------------------

function renderAgentPage(
  path: "monitor" | "traces",
  deployment?: AgentDeployment,
  initialEntry?: string,
) {
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
          { path: "traces", Component: AgentTraces },
        ],
      },
    ],
    {
      initialEntries: [initialEntry ?? `/testuser/agents/${dep.id}/${path}`],
      auth: mockAuthContext,
    },
  );

  return { ...result, user, getLocation: () => currentLocation };
}

function renderMonitor(deployment?: AgentDeployment, initialEntry?: string) {
  return renderAgentPage("monitor", deployment, initialEntry);
}

function renderTraces(deployment?: AgentDeployment, initialEntry?: string) {
  return renderAgentPage("traces", deployment, initialEntry);
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

describe("legacy trace links", () => {
  it.each([
    ["?trace=legacy-trace&window=14d", "?trace=legacy-trace&window=14d"],
    ["?window=14d#traces", "?window=14d"],
  ])("redirects monitor%s to traces%s", async (source, target) => {
    setupHandlers();
    const { getLocation } = renderMonitor(
      undefined,
      `/testuser/agents/dep-1/monitor${source}`,
    );

    await waitFor(() => {
      expect(getLocation()).toBe(`/testuser/agents/dep-1/traces${target}`);
    });
  });
});

describe("user views traces", () => {
  it("shows traces table with latency, cost, and trace ID", async () => {
    setupHandlers(emptyMetrics, {
      traces: [
        makeTrace({ trace_id: "abc-123", latency_ms: 245, total_cost: 0.0012 }),
        makeTrace({ trace_id: "def-456", latency_ms: 1500, total_cost: 0.05 }),
      ],
      total: 2,
      limit: 100,
      offset: 0,
    });
    renderTraces();
    // Wait for trace data to arrive (trace ID is data-specific, not a static label)
    expect(await screen.findByText("abc-123")).toBeInTheDocument();
    expect(screen.getByText("def-456")).toBeInTheDocument();
    expect(screen.getByText("245ms")).toBeInTheDocument();
    expect(screen.getByText("1.5s")).toBeInTheDocument();
    expect(screen.getByText("2 traces")).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "Status" })).not.toBeInTheDocument();
  });

  it("shows no-traces message when response is empty", async () => {
    setupHandlers();
    renderTraces();
    expect(await screen.findByText("No traces found.")).toBeInTheDocument();
  });
});

describe("user loads more traces", () => {
  it("shows more traces and can collapse back to the first 100", async () => {
    const all = Array.from({ length: 105 }, (_, i) =>
      makeTrace({ trace_id: `trace-${String(i).padStart(3, "0")}` }),
    );
    const offsets: number[] = [];
    server.use(
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        const offset = Number(
          new URL(request.url).searchParams.get("offset") ?? "0",
        );
        offsets.push(offset);
        const pageSize = 100;
        return HttpResponse.json({
          traces: all.slice(offset, offset + pageSize),
          total: all.length,
          limit: pageSize,
          offset,
        });
      }),
    );
    renderTraces();
    fireEvent.click(await screen.findByRole("button", { name: "Show more" }));
    await waitFor(() => expect(offsets).toEqual([0, 100]));

    fireEvent.click(screen.getByRole("button", { name: "Show less" }));
    expect(await screen.findByText("100 shown · 105 of 105 loaded")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show more" })).toBeInTheDocument();
  }, 10_000);

  it("paginates within the complete filtered result", async () => {
    const all = Array.from({ length: 155 }, (_, index) => {
      const isJanet = index < 105;
      return makeTrace({
        trace_id: `${isJanet ? "janet" : "other"}-${index}`,
        user_id: isJanet ? "janet" : "other",
        user_details: {
          kind: "astro",
          display_name: isJanet ? "Janet" : "Other User",
        },
      });
    });
    const janetOffsets: number[] = [];
    server.use(
      traceUserFacetsHandler(all),
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        const params = new URL(request.url).searchParams;
        const offset = Number(params.get("offset") ?? "0");
        const limit = Number(params.get("limit") ?? "100");
        const filtered = params.get("user_id") === "janet"
          ? all.filter((trace) => trace.user_id === "janet")
          : all;
        if (params.get("user_id") === "janet") janetOffsets.push(offset);
        return HttpResponse.json({
          traces: filtered.slice(offset, offset + limit),
          total: filtered.length,
          limit,
          offset,
        });
      }),
    );
    renderTraces();

    fireEvent.click(await screen.findByRole("button", { name: /filter by user/i }));
    fireEvent.click(screen.getByRole("button", { name: /Janet/i }));
    expect(await screen.findByText("100 shown · 100 of 105 loaded")).toBeInTheDocument();
    expect(screen.queryByText("janet-104")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(await screen.findByText("janet-104")).toBeInTheDocument();
    expect(screen.getByText("105 traces")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
    expect(janetOffsets).toEqual([0, 100]);
  }, 15_000);

  it("searches the complete trace window, including unloaded pages", async () => {
    const all = [
      makeTrace({ trace_id: "loaded-first" }),
      makeTrace({ trace_id: "loaded-second" }),
      makeTrace({ trace_id: "match-2", name: "needle-span" }),
      makeTrace({ trace_id: "loaded-last" }),
    ];
    server.use(
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        const params = new URL(request.url).searchParams;
        const offset = Number(params.get("offset") ?? "0");
        const limit = 2;
        const matching = params.get("search") === "needle"
          ? all.filter((trace) => trace.name.includes("needle"))
          : all;
        return HttpResponse.json({
          traces: matching.slice(offset, offset + limit),
          total: matching.length,
          limit,
          offset,
        });
      }),
    );
    const { user } = renderTraces();
    await screen.findByText("loaded-first");

    await user.type(screen.getByRole("textbox", { name: /search traces/i }), "needle");

    expect(await screen.findByText("match-2")).toBeInTheDocument();
    expect(screen.queryByText("loaded-first")).not.toBeInTheDocument();
    expect(screen.getByText("1 trace")).toBeInTheDocument();
  });

  it("does not load every page merely from opening the user filter", async () => {
    const all = [
      makeTrace({
        trace_id: "ada-trace",
        user_id: "ada",
        user_details: { kind: "astro", display_name: "Ada Lovelace" },
      }),
      makeTrace({
        trace_id: "bob-trace",
        user_id: "bob",
        user_details: { kind: "astro", display_name: "Bob Stone" },
      }),
    ];
    let requestCount = 0;
    server.use(
      traceUserFacetsHandler(all),
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        requestCount += 1;
        const offset = Number(
          new URL(request.url).searchParams.get("offset") ?? "0",
        );
        const limit = 1;
        return HttpResponse.json({
          traces: all.slice(offset, offset + limit),
          total: all.length,
          limit,
          offset,
        });
      }),
    );
    const { user } = renderTraces();
    await screen.findByText("ada-trace");

    await user.click(screen.getByRole("button", { name: /filter by user/i }));
    await user.type(screen.getByPlaceholderText("Filter users"), "Bob");

    expect(screen.getByRole("button", { name: /Bob Stone/i })).toBeInTheDocument();
    expect(requestCount).toBe(1);

    await user.clear(screen.getByPlaceholderText("Filter users"));
    await user.click(screen.getByRole("button", { name: /Ada Lovelace/i }));
    await user.click(screen.getByRole("button", { name: /filter by user/i }));
    expect(screen.getByRole("button", { name: /Bob Stone/i })).toBeInTheDocument();
  });

  it("requests server ordering before paginating a non-default sort", async () => {
    const all = [
      makeTrace({
        trace_id: "newest",
        latency_ms: 300,
        timestamp: "2026-07-14T04:00:00Z",
      }),
      makeTrace({
        trace_id: "newer",
        latency_ms: 200,
        timestamp: "2026-07-14T03:00:00Z",
      }),
      makeTrace({
        trace_id: "global-min",
        latency_ms: 1,
        timestamp: "2026-07-14T02:00:00Z",
      }),
      makeTrace({
        trace_id: "oldest",
        latency_ms: 100,
        timestamp: "2026-07-14T01:00:00Z",
      }),
    ];
    server.use(
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        const params = new URL(request.url).searchParams;
        const offset = Number(params.get("offset") ?? "0");
        const limit = 2;
        const ordered = [...all].sort((a, b) => {
          if (params.get("sort") === "latency") {
            const comparison = a.latency_ms - b.latency_ms;
            return params.get("direction") === "asc" ? comparison : -comparison;
          }
          return b.timestamp.localeCompare(a.timestamp);
        });
        return HttpResponse.json({
          traces: ordered.slice(offset, offset + limit),
          total: all.length,
          limit,
          offset,
        });
      }),
    );
    const { user } = renderTraces();

    expect(await screen.findByText("2 shown · 2 of 4 loaded")).toBeInTheDocument();
    expect(screen.queryByText("global-min")).not.toBeInTheDocument();

    // A new sort key starts descending and the server returns its first page.
    await user.click(screen.getByRole("button", { name: /sort by latency/i }));
    expect(await screen.findByText("newest")).toBeInTheDocument();
    expect(screen.queryByText("global-min")).not.toBeInTheDocument();

    // Toggling to ascending starts a new server-ordered result at the global minimum.
    await user.click(screen.getByRole("button", { name: /sort by latency/i }));
    await waitFor(() => {
      expect(document.querySelector("tbody tr")?.textContent).toContain("global-min");
    });
  });
});

describe("user inspects trace details", () => {
  function setupDetailTraces() {
    setupHandlers(emptyMetrics, {
      traces: [
        makeTrace({
          trace_id: "t-1",
          latency_ms: 245,
          total_cost: 0.0012,
          input: "What is the weather?",
          output: "It is sunny today.",
        }),
        makeTrace({
          trace_id: "t-2",
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

  it("clicking a trace row opens the detail panel with latency and cost", async () => {
    setupDetailTraces();
    const { user, getLocation } = renderTraces();
    await user.click((await screen.findByText("t-1")).closest("tr")!);

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).queryByText("Status")).not.toBeInTheDocument();
    expect(within(panel).getByText("245ms")).toBeInTheDocument();
    expect(within(panel).getByText("$0.0012")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLocation()).toBe("/testuser/agents/dep-1/traces?trace=t-1");
    });
  });

  it("opens the detail panel from the trace query parameter", async () => {
    setupDetailTraces();
    renderTraces(undefined, "/testuser/agents/dep-1/traces?trace=t-2");

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(within(panel).getByText("t-2")).toBeInTheDocument();
    expect(await within(panel).findByText("Complex query")).toBeInTheDocument();
  });

  it("expands and restores the trace detail panel", async () => {
    setupDetailTraces();
    const { user } = renderTraces();
    await user.click((await screen.findByText("t-1")).closest("tr")!);

    await user.click(screen.getByRole("button", { name: /expand panel to full width/i }));
    expect(screen.getByRole("button", { name: /restore panel size/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /restore panel size/i }));
    expect(
      screen.getByRole("button", { name: /expand panel to full width/i }),
    ).toBeInTheDocument();
  });

  it("overlays the table below the responsive threshold", async () => {
    containerSize.width = 800;
    setupDetailTraces();
    const { user } = renderTraces();
    await user.click((await screen.findByText("t-1")).closest("tr")!);

    const panel = await screen.findByRole("dialog", { name: /trace details/i });
    expect(panel.parentElement).toHaveClass("inset-3");
    expect(screen.queryByRole("button", { name: /expand panel/i })).not.toBeInTheDocument();
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
    renderTraces(
      undefined,
      `/testuser/agents/dep-1/traces?trace=${targetTraceId}#${traceRowAnchorId(targetTraceId)}`,
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
    const { user } = renderTraces();

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
    const { user } = renderTraces();
    await user.click((await screen.findByText("t-1")).closest("tr")!);

    expect(await screen.findByText("Input")).toBeInTheDocument();
    expect(screen.getByText("Output")).toBeInTheDocument();
    expect(screen.getByText("What is the weather?")).toBeInTheDocument();
    expect(screen.getByText("It is sunny today.")).toBeInTheDocument();
  });

  it("shows 'No content.' when output is empty", async () => {
    setupDetailTraces();
    const { user } = renderTraces();
    // Click the error trace with empty output
    await user.click((await screen.findByText("t-2")).closest("tr")!);
    expect(await screen.findByText("No content.")).toBeInTheDocument();
  });

  it("close button dismisses the panel", async () => {
    setupDetailTraces();
    const { user, getLocation } = renderTraces(undefined, "/testuser/agents/dep-1/traces?trace=t-1");
    expect(await screen.findByText("What is the weather?")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /close trace/i }));
    await waitFor(() => {
      expect(screen.queryByText("What is the weather?")).not.toBeInTheDocument();
    });
    expect(getLocation()).toBe("/testuser/agents/dep-1/traces");
  });

  it("clears a selected trace when closing a clicked row", async () => {
    setupDetailTraces();
    const { user, getLocation } = renderTraces();
    await user.click((await screen.findByText("t-1")).closest("tr")!);
    expect(await screen.findByText("What is the weather?")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /close trace/i }));
    await waitFor(() => {
      expect(screen.queryByText("What is the weather?")).not.toBeInTheDocument();
    });
    expect(getLocation()).toBe("/testuser/agents/dep-1/traces");
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
    const { user } = renderTraces();
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
    const { user, getLocation } = renderTraces();
    await user.click((await screen.findByText("first")).closest("tr")!);
    expect(await screen.findByText("First question")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLocation()).toBe("/testuser/agents/dep-1/traces?trace=first");
    });

    await user.click(screen.getByRole("button", { name: /next trace/i }));
    expect(await screen.findByText("Second question")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLocation()).toBe("/testuser/agents/dep-1/traces?trace=second");
    });
  });

  it("Prev is disabled on the first trace", async () => {
    setupNavTraces();
    const { user } = renderTraces();
    await user.click((await screen.findByText("first")).closest("tr")!);
    await screen.findByText("First question");

    expect(screen.getByRole("button", { name: /previous trace/i })).toBeDisabled();
  });

  it("Next is disabled on the last trace", async () => {
    setupNavTraces();
    const { user } = renderTraces();
    await user.click((await screen.findByText("third")).closest("tr")!);
    await screen.findByText("Third question");

    expect(screen.getByRole("button", { name: /next trace/i })).toBeDisabled();
  });

  it("navigates in the same filtered and sorted order shown by the table", async () => {
    const traces = [
      makeTrace({
          trace_id: "alpha",
          latency_ms: 100,
          input: "Alpha question",
          output: "Alpha answer",
          user_id: "user-ada",
          user_details: { kind: "astro", display_name: "Ada Lovelace", username: "ada" },
      }),
      makeTrace({
          trace_id: "beta",
          latency_ms: 900,
          input: "Beta question",
          output: "Beta answer",
          user_id: "user-bob",
          user_details: { kind: "astro", display_name: "Bob Stone", username: "bob" },
      }),
      makeTrace({
          trace_id: "gamma",
          latency_ms: 500,
          input: "Gamma question",
          output: "Gamma answer",
          user_id: "user-ada",
          user_details: { kind: "astro", display_name: "Ada Lovelace", username: "ada" },
      }),
    ];
    setupHandlers(emptyMetrics);
    server.use(
      traceUserFacetsHandler(traces),
      http.get("/api/v1/deployments/:id/observability/traces", ({ request }) => {
        const params = new URL(request.url).searchParams;
        const filtered = params.get("user_id") === "user-ada"
          ? traces.filter((trace) => trace.user_id === "user-ada")
          : traces;
        const ordered = params.get("sort") === "latency"
          ? [...filtered].sort((a, b) => b.latency_ms - a.latency_ms)
          : filtered;
        return HttpResponse.json({
          traces: ordered,
          total: ordered.length,
          limit: 100,
          offset: 0,
        });
      }),
    );
    const { user } = renderTraces();

    await user.click(await screen.findByRole("button", { name: /filter by user/i }));
    await user.click(screen.getByRole("button", { name: /Ada Lovelace/i }));
    expect(screen.queryByText("beta")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /sort by latency/i }));
    await user.click(screen.getByText("gamma").closest("tr")!);
    expect(await screen.findByText("Gamma question")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /next trace/i }));
    expect(await screen.findByText("Alpha question")).toBeInTheDocument();
  });
});
