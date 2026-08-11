import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, fireEvent, screen, cleanup, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { Outlet } from "react-router";
import { server } from "@/test/msw/server";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import type {
  AgentDeployment,
  WorkloadDetail,
  ContainerStatus,
  DeploymentHistoryResponse,
  DeploymentEventsResponse,
  DeploymentRuntime,
} from "@/lib/api";
import type { LogEntry } from "@/lib/log-utils";
import AgentDeployments from "./AgentDeployments";

const { mockScrollToOffset } = vi.hoisted(() => ({
  mockScrollToOffset: vi.fn(),
}));

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// PodGraph depends on ResizeObserver (via useContainerSize) and
// getBoundingClientRect (via useTileMeasurements) — neither works in jsdom.
// Mock both hooks so pods reach the "ready" state without layout.
vi.mock("@/hooks/use-container-size", () => ({
  useContainerSize: () => ({ ref: { current: null }, width: 1200, height: 800 }),
}));

vi.mock("@/components/agent-detail/pods/use-tile-measurements", () => ({
  useTileMeasurements: (count: number) => ({
    sizes: Array.from({ length: count }, () => ({ width: 200, height: 120 })),
    measureRef: () => () => {},
  }),
}));

// Virtualizer — jsdom has no layout engine. Render all rows directly.
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (opts: { count: number; getItemKey?: (index: number) => number | string | bigint }) => ({
    getVirtualItems: () =>
      Array.from({ length: opts.count }, (_, i) => ({
        key: opts.getItemKey?.(i) ?? i,
        index: i,
        start: i * 24,
        end: (i + 1) * 24,
        size: 24,
      })),
    getTotalSize: () => opts.count * 24,
    getOffsetForIndex: (index: number) => [index * 24, "start"] as const,
    measureElement: vi.fn(),
    scrollToIndex: vi.fn(),
    scrollToOffset: mockScrollToOffset,
  }),
}));

// LogStreamProvider — mock the context so PodLogsTab renders without a real
// WebSocket. Individual tests can override useLogStream behavior.
const mockStartStream = vi.fn();
const mockStopStream = vi.fn();
vi.mock("@/lib/log-stream", () => ({
  LogStreamProvider: ({ children }: { children: React.ReactNode }) => children,
  useLogStream: () => ({
    lines: [],
    status: "idle" as const,
    error: undefined,
    startStream: mockStartStream,
    stopStream: mockStopStream,
  }),
}));

// Default MSW handlers — tests override with server.use() AFTER these.
beforeEach(() => {
  server.use(
    http.get("/api/v1/agents/:account/:name/deployment/history", () =>
      HttpResponse.json<DeploymentHistoryResponse>({
        deployments: [makeHistoryRecord()],
        count: 1,
      }),
    ),
    http.get("/api/v1/deployments/:id/logs", () => HttpResponse.json<LogEntry[]>([])),
    http.get("/api/v1/deployments/:id/events", () =>
      HttpResponse.json<DeploymentEventsResponse>({ events: [] }),
    ),
    http.get("/api/v1/agents/:account/:name/github", () =>
      HttpResponse.json({
        connected: false,
        repo_full_name: "",
        branch: "",
        builds: [],
      }),
    ),
  );
});

afterEach(cleanup);
afterEach(() => {
  server.resetHandlers();
  mockStartStream.mockClear();
  mockStopStream.mockClear();
  mockScrollToOffset.mockClear();
});

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

function makeContainer(overrides?: Partial<ContainerStatus>): ContainerStatus {
  return {
    name: "app",
    state: "running",
    ready: true,
    restart_count: 0,
    ...overrides,
  };
}

function makeWorkload(overrides?: Partial<WorkloadDetail>): WorkloadDetail {
  return {
    name: "my-agent-7f8d9c-xk2lp",
    kind: "Pod",
    component: "agent",
    age: "3h",
    pod_name: "my-agent-7f8d9c-xk2lp",
    containers: [makeContainer()],
    urls: [{ name: "http", url: "https://agent.example.com", type: "http" }],
    // Env now lives on the workload, keyed by role. Default agent workload
    // gets the "agent" role's env; PodDetailPanel looks it up via
    // roleFor(component, container.name).
    env: {
      agent: [
        { name: "PORT", value: "8080" },
        { name: "OPENAI_API_KEY", value: "••••••••", is_secret: true, source: "user_var" },
      ],
    },
    ...overrides,
  };
}

function makeDeployment(overrides?: Partial<AgentDeployment>): AgentDeployment {
  return {
    id: "dep-1",
    name: "my-agent",
    display_name: "My Agent",
    build_id: "a1b2c3d4",
    namespace: "astro-ns",
    status: "Running",
    replicas: 1,
    created_at: "2025-04-01T00:00:00Z",
    components: ["agent"],
    workloads: [makeWorkload()],
    external_urls: [{ name: "http", url: "https://public.example.com", type: "http" }],
    avatar_colors: { accent: "#2dd4bf", base: "#0f766e", vibrant: "#2dd4bf", vibrant_light: "#5eead4", accent_light: "#99f6e4", background: "#042f2e", foreground: "#f0fdfa", glow: "#2dd4bf" },
    ...overrides,
  };
}

/**
 * Default runtime view for a deployment: assumes the cluster has reached the
 * desired state (ready = replicas) and lifts the joined workload entries'
 * runtime fields (containers, age, pod_name, status) into a WorkloadRuntime
 * keyed by name. Tests that need a transitional or degraded runtime build a
 * `DeploymentRuntime` literal directly and pass it as the second arg to
 * `renderDeployments`.
 */
function defaultRuntimeFor(dep: AgentDeployment): DeploymentRuntime {
  return {
    ready: dep.replicas,
    replicas: dep.replicas,
    messaging_reachable: true,
    workloads: (dep.workloads ?? []).map((w) => {
      const wd = w as WorkloadDetail;
      return {
        name: wd.name,
        age: wd.age,
        phase: wd.phase,
        pod_name: wd.pod_name,
        containers: wd.containers,
        status: wd.status,
        start_time: wd.start_time,
        completions: wd.completions,
        runs: wd.runs,
      };
    }),
  };
}

function makeHistoryRecord(overrides?: Partial<import("@/lib/api").DeploymentHistoryRecord>) {
  return {
    id: "dep-1",
    agent_name: "my-agent",
    revision: 1,
    build_id: "a1b2c3d4",
    namespace: "astro-ns",
    display_name: "My Agent",
    is_current: true,
    status: "Running",
    deployed_at: "2025-04-01T00:00:00Z",
    spec: {},
    source: "github" as const,
    branch: "main",
    commit_message: "Fix auth flow",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Rendering helper
// ---------------------------------------------------------------------------

/**
 * Renders the AgentDeployments page inside a minimal layout that provides
 * the outlet context AgentDetail normally supplies.
 */
function renderDeployments(
  deployment?: AgentDeployment,
  runtime?: DeploymentRuntime,
  options?: { probing?: boolean },
) {
  const dep = deployment ?? makeDeployment();
  // Default runtime: assume the deployment is fully observed (ready === replicas)
  // unless the test overrides. Keeps "Active" the default for tests that don't
  // care about the runtime split.
  const rt = options?.probing ? undefined : runtime ?? defaultRuntimeFor(dep);
  const user = userEvent.setup();

  const result = renderRoute(
    [
      {
        path: "/:account/agents/:deploymentId",
        Component: () => (
          <Outlet
            context={{
              deployment: dep,
              runtime: rt,
              account: "testuser",
              deploymentId: dep.id,
            }}
          />
        ),
        children: [
          {
            path: "deployments",
            Component: AgentDeployments,
          },
          {
            path: "configure",
            Component: () => <div data-testid="configure-page">Configure</div>,
          },
        ],
      },
    ],
    {
      initialEntries: [`/testuser/agents/${dep.id}/deployments`],
      auth: mockAuthContext,
    },
  );

  return { ...result, user };
}

// ===========================================================================
// Tests
// ===========================================================================

describe("user views running pods", () => {
  it("shows pod tiles with component name and healthy status", async () => {
    renderDeployments();
    expect(await screen.findByText("agent")).toBeInTheDocument();
    expect(screen.getByText("Online")).toBeInTheDocument();
  });

  it("shows multiple pods when deployment has multiple workloads", async () => {
    renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload({ component: "agent" }),
          makeWorkload({ name: "redis-abc", component: "redis", pod_name: "redis-abc" }),
        ],
      }),
    );
    expect(await screen.findByText("agent")).toBeInTheDocument();
    expect(screen.getByText("redis")).toBeInTheDocument();
  });

  it("shows empty state when deployment has no workloads", async () => {
    renderDeployments(makeDeployment({ workloads: [] }));
    expect(await screen.findByText("No active pods")).toBeInTheDocument();
  });

  it("shows a deploying state, not 'No active pods', while a deploy is in flight with no pods yet (#1876)", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({ value: "deploying", reason: "provisioning", details: "Pods are being provisioned" }),
      ),
    );
    renderDeployments(makeDeployment({ workloads: [] }));
    expect(await screen.findByText("Deploying your agent…")).toBeInTheDocument();
    expect(screen.queryByText("No active pods")).not.toBeInTheDocument();
  });

  it("shows Error status and last error message on unhealthy pod", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/logs", () =>
        HttpResponse.json<LogEntry[]>([
          { timestamp: "2025-04-01T00:00:00Z", level: "error", message: "OOMKilled" },
        ]),
      ),
    );
    renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload({
            containers: [makeContainer({ state: "waiting", ready: false })],
          }),
        ],
      }),
    );
    expect(await screen.findByText("Error")).toBeInTheDocument();
    expect(await screen.findByText("OOMKilled")).toBeInTheDocument();
  });

  it("keeps spec workload tiles visible when runtime reports zero containers", async () => {
    // The spec workload list is the stable source of truth — tiles never
    // disappear mid-transition (pause/resume window). When runtime shows
    // no containers and the deployment isn't paused, derivePodStatus
    // surfaces "Starting" until pods come up, but the tile itself stays
    // mounted to avoid flicker.
    renderDeployments(
      makeDeployment({
        workloads: [makeWorkload({ containers: [] })],
      }),
    );
    expect(await screen.findByText("Starting")).toBeInTheDocument();
    expect(screen.queryByText("No active pods")).not.toBeInTheDocument();
  });
});

describe("user inspects pod details", () => {
  it("clicking a pod opens the detail panel with pod name and status", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    // Panel header shows the component name — the tile also has it, so find the h2
    expect(await screen.findByRole("heading", { level: 2 })).toHaveTextContent("agent");
    // "Online" appears on both the tile and the panel badge
    expect(screen.getAllByText("Online").length).toBeGreaterThanOrEqual(2);
  });

  it("keeps the detail panel status paused when the deployment is paused", async () => {
    const { user } = renderDeployments(
      makeDeployment({
        status: "stopped",
        workloads: [makeWorkload({ containers: [] })],
      }),
    );

    await user.click(await screen.findByText("agent"));

    expect(await screen.findByRole("heading", { level: 2 })).toHaveTextContent("agent");
    expect(screen.getAllByText("Paused").length).toBeGreaterThanOrEqual(2);
    expect(screen.queryByText("Starting")).not.toBeInTheDocument();
  });

  it("General tab shows Domains and Containers sections", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("Domains")).toBeInTheDocument();
    expect(screen.getByText("Containers")).toBeInTheDocument();
    expect(screen.getByText("https://agent.example.com")).toBeInTheDocument();
  });

  it("shows external URLs for agent component pods", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("https://public.example.com")).toBeInTheDocument();
  });

  it("dedupes URLs shared between deployment external URLs and the agent workload's own service endpoints", async () => {
    const { user } = renderDeployments(
      makeDeployment({
        components: ["agent", "frontend"],
        external_urls: [
          { name: "agent", url: "https://agent.example.com", type: "http" },
          { name: "frontend", url: "https://frontend.example.com", type: "http" },
        ],
        workloads: [
          makeWorkload({
            urls: [{ name: "http", url: "https://agent.example.com", type: "http" }],
          }),
        ],
      }),
    );
    await user.click(await screen.findByText("agent"));
    await screen.findByText("Domains");
    expect(screen.getAllByText("https://agent.example.com")).toHaveLength(1);
    expect(screen.getByText("https://frontend.example.com")).toBeInTheDocument();
  });

  it("masks sensitive environment variables", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("OPENAI_API_KEY")).toBeInTheDocument();
    expect(screen.getByText("••••••••")).toBeInTheDocument();
    expect(screen.queryByText("sk-test-123")).not.toBeInTheDocument();
    expect(screen.getByText("8080")).toBeInTheDocument();
  });

  it("shows empty states when no URLs or env vars exist", async () => {
    const { user } = renderDeployments(
      makeDeployment({
        external_urls: [],
        workloads: [
          makeWorkload({
            urls: [],
            env: {},
          }),
        ],
      }),
    );
    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("No domains configured")).toBeInTheDocument();
  });

  it("clicking close dismisses the detail panel", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("Domains")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /close pod details/i }));
    await waitFor(() => {
      expect(screen.queryByText("Domains")).not.toBeInTheDocument();
    });
  });

  it("Restart Pod button calls restart on the correct deployment and pod", async () => {
    const restartHandler = vi.fn();
    server.use(
      http.post("/api/v1/deployments/:id/pods/:podName/restart", ({ params }) => {
        restartHandler(params);
        return HttpResponse.json({ status: "ok", pod: params.podName });
      }),
    );
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(await screen.findByRole("button", { name: /restart pod/i }));
    await waitFor(() =>
      expect(restartHandler).toHaveBeenCalledWith({
        id: "dep-1",
        podName: "my-agent-7f8d9c-xk2lp",
      }),
    );
  });

  it("hides Danger Zone when pod has no pod_name", async () => {
    const { user } = renderDeployments(
      makeDeployment({
        workloads: [makeWorkload({ pod_name: undefined })],
      }),
    );
    await user.click(await screen.findByText("agent"));
    await screen.findByText("Domains");
    expect(screen.queryByText("Danger Zone")).not.toBeInTheDocument();
  });
});

describe("startup failure diagnostics", () => {
  it("opens the first errored workload on General when no error logs are available", async () => {
    const logsRequested = vi.fn();
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Workloads failed to start",
        }),
      ),
      http.get("/api/v1/deployments/:id/logs", () => {
        logsRequested();
        return HttpResponse.json<LogEntry[]>([]);
      }),
    );

    renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload(),
          makeWorkload({
            name: "redis-abc",
            component: "redis",
            pod_name: "redis-abc-pod",
            containers: [
              makeContainer({ state: "waiting", ready: false }),
            ],
          }),
          makeWorkload({
            name: "worker-abc",
            component: "worker",
            pod_name: "worker-abc-pod",
            containers: [
              makeContainer({ state: "terminated", ready: false }),
            ],
          }),
        ],
      }),
    );

    expect(
      await screen.findByRole("heading", { level: 2, name: "redis" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(logsRequested).toHaveBeenCalled());
    expect(screen.getByText("Domains")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "General" })).toHaveClass(
      "text-foreground",
    );
    expect(screen.queryByText("No logs in this time window")).not.toBeInTheDocument();
  });

  it("keeps the failed workload on General when an error log is available", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Agent failed to start",
          failed_on: [
            {
              workload: "my-agent-7f8d9c-xk2lp",
              component: "agent",
              phase: "failed",
            },
          ],
        }),
      ),
      http.get("/api/v1/deployments/:id/logs", () =>
        HttpResponse.json<LogEntry[]>([
          {
            timestamp: "2025-04-01T00:00:00Z",
            level: "error",
            message: "Connection refused",
          },
        ]),
      ),
    );

    renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload({
            kind: "Deployment",
            containers: [
              makeContainer({ state: "terminated", ready: false }),
            ],
          }),
        ],
      }),
    );

    expect(await screen.findByText("Errors in logs")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "General" })).toHaveClass(
      "text-foreground",
    );
    expect(screen.getByText("Domains")).toBeInTheDocument();
  });

  it("prefers the failed_on workload when multiple workloads are errored", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Worker failed to start",
          failed_on: [
            {
              workload: "worker-abc",
              component: "worker",
              phase: "failed",
              title: "Worker failed to start",
              guidance: "Inspect the worker logs.",
            },
          ],
        }),
      ),
    );

    renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload({
            name: "redis-abc",
            component: "redis",
            pod_name: "redis-abc-pod",
            containers: [
              makeContainer({ state: "waiting", ready: false }),
            ],
          }),
          makeWorkload({
            name: "worker-abc",
            component: "worker",
            pod_name: "worker-abc-pod",
            containers: [
              makeContainer({ state: "terminated", ready: false }),
            ],
          }),
        ],
      }),
    );

    expect(
      await screen.findByRole("heading", { level: 2, name: "worker" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Domains")).toBeInTheDocument();
    expect(screen.queryByText("No logs in this time window")).not.toBeInTheDocument();
  });

  it("does not open a pod panel when no workloads are errored", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Deployment failed",
        }),
      ),
    );

    renderDeployments();

    expect(
      await screen.findByText("Deployment failed"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /close pod details/i }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("No logs in this time window")).not.toBeInTheDocument();
  });

  it("does not open a pod panel while runtime is probing", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Deployment failed",
        }),
      ),
    );

    renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload({
            containers: [
              makeContainer({ state: "waiting", ready: false }),
            ],
          }),
        ],
      }),
      undefined,
      { probing: true },
    );

    expect(await screen.findByText("Deployment failed")).toBeInTheDocument();
    expect(screen.getByText("Probing")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /close pod details/i }),
    ).not.toBeInTheDocument();
  });

  it("does not open a pod panel for a paused deployment", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Deployment failed",
        }),
      ),
    );

    renderDeployments(
      makeDeployment({
        status: "stopped",
        workloads: [
          makeWorkload({
            containers: [
              makeContainer({ state: "waiting", ready: false }),
            ],
          }),
        ],
      }),
    );

    expect(await screen.findByText("Deployment failed")).toBeInTheDocument();
    expect(screen.getByText("Paused")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /close pod details/i }),
    ).not.toBeInTheDocument();
  });

  it("preserves a manual pod selection when a failure surfaces", async () => {
    let releaseStatus!: () => void;
    const statusGate = new Promise<void>((resolve) => {
      releaseStatus = resolve;
    });
    server.use(
      http.get("/api/v1/deployments/:id/status", async () => {
        await statusGate;
        return HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Deployment failed",
        });
      }),
    );

    const { user } = renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload(),
          makeWorkload({
            name: "redis-abc",
            component: "redis",
            pod_name: "redis-abc-pod",
            containers: [
              makeContainer({ state: "waiting", ready: false }),
            ],
          }),
        ],
      }),
    );

    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("Domains")).toBeInTheDocument();

    releaseStatus();
    expect(await screen.findByText("Deployment failed")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 2, name: "agent" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Domains")).toBeInTheDocument();
    expect(screen.queryByText("No logs in this time window")).not.toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: /close pod details/i }),
    );
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /close pod details/i }),
      ).not.toBeInTheDocument();
    });
    expect(screen.queryByText("No logs in this time window")).not.toBeInTheDocument();
  });

  it("does not reopen failed pod details after the user closes the panel", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({
          value: "error",
          reason: "failed",
          details: "Deployment failed",
          failed_on: [
            {
              workload: "my-agent-7f8d9c-xk2lp",
              phase: "failed",
            },
          ],
        }),
      ),
    );

    const { user } = renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload({
            containers: [
              makeContainer({ state: "waiting", ready: false }),
            ],
          }),
        ],
      }),
    );
    await screen.findByText("Domains");
    await user.click(
      screen.getByRole("button", { name: /close pod details/i }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("heading", { level: 2, name: "agent" }),
      ).not.toBeInTheDocument();
    });
    expect(screen.queryByText("Domains")).not.toBeInTheDocument();
  });
});

describe("user views pod events", () => {
  it("Events tab shows K8s events with reason and message", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/events", () =>
        HttpResponse.json<DeploymentEventsResponse>({
          events: [
            {
              type: "Normal",
              reason: "Scheduled",
              message: "Assigned to node-1",
              object_kind: "Pod",
              object_name: "my-agent-abc",
              count: 1,
              first_timestamp: "2025-04-01T00:00:00Z",
              last_timestamp: "2025-04-01T00:00:00Z",
            },
            {
              type: "Warning",
              reason: "Unhealthy",
              message: "Readiness probe failed",
              object_kind: "Pod",
              object_name: "my-agent-abc",
              count: 3,
              first_timestamp: "2025-04-01T00:00:10Z",
              last_timestamp: "2025-04-01T00:00:30Z",
            },
          ],
        }),
      ),
    );
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Events" }));
    expect(await screen.findByText("Scheduled")).toBeInTheDocument();
    const unhealthy = screen.getByText("Unhealthy");
    expect(unhealthy).toBeInTheDocument();
    // The raw message lives in the expanded row detail.
    await user.click(unhealthy);
    expect(screen.getByText("Readiness probe failed")).toBeInTheDocument();
  });

  it("shows empty state when no events exist", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Events" }));
    expect(await screen.findByText("No events")).toBeInTheDocument();
  });
});

describe("user views and searches logs", () => {
  function setupLogs(logs: LogEntry[]) {
    server.use(
      http.get("/api/v1/deployments/:id/logs", () => HttpResponse.json<LogEntry[]>(logs)),
    );
  }

  function makeLogPage(messagePrefix: string, hour: number, minute = 0): LogEntry[] {
    return Array.from({ length: 500 }, (_, i) => ({
      timestamp: new Date(Date.UTC(2025, 3, 1, hour, minute, i)).toISOString(),
      level: "info",
      message: `${messagePrefix} ${i}`,
    }));
  }

  it("shows historical log lines with messages", async () => {
    setupLogs([
      { timestamp: "2025-04-01T12:00:01Z", level: "info", message: "Agent started" },
      { timestamp: "2025-04-01T12:00:02Z", level: "error", message: "Connection refused" },
    ]);
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Logs" }));
    expect(await screen.findByText("Agent started")).toBeInTheDocument();
    expect(screen.getByText("Connection refused")).toBeInTheDocument();
  });

  it("shows empty state when no logs exist", async () => {
    setupLogs([]);
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Logs" }));
    expect(await screen.findByText("No logs in this time window")).toBeInTheDocument();
  });

  it("searching logs filters to matching lines", async () => {
    setupLogs([
      { timestamp: "2025-04-01T12:00:01Z", level: "info", message: "Agent started" },
      { timestamp: "2025-04-01T12:00:02Z", level: "info", message: "Processing request abc" },
      { timestamp: "2025-04-01T12:00:03Z", level: "info", message: "Agent stopped" },
    ]);
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Logs" }));
    await screen.findByText("Agent started");

    await user.type(screen.getByPlaceholderText("Search"), "request");
    // Search highlight wraps the match in <mark>, so use a function matcher
    await waitFor(() => {
      expect(screen.queryByText("Agent started")).not.toBeInTheDocument();
      expect(screen.queryByText("Agent stopped")).not.toBeInTheDocument();
    });
    // The matching line is still present (with highlighted "request" in a <mark>)
    expect(screen.getByText((_, el) => el?.textContent === "Processing request abc")).toBeInTheDocument();
  });

  it("loads older logs on request and keeps the current log anchored", async () => {
    const firstPage = makeLogPage("line", 12);
    const olderPage = makeLogPage("older line", 11, 50);
    let callCount = 0;
    let resolveOlderPage: (() => void) | undefined;

    server.use(
      http.get("/api/v1/deployments/:id/logs", () => {
        callCount++;
        if (callCount === 1) return HttpResponse.json<LogEntry[]>(firstPage);
        if (callCount > 2) return HttpResponse.json<LogEntry[]>([]);
        return new Promise<Response>((resolve) => {
          resolveOlderPage = () => resolve(HttpResponse.json<LogEntry[]>(olderPage));
        });
      }),
    );

    const { container, user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Logs" }));
    await screen.findByText("line 99");

    const scroll = Array.from(container.querySelectorAll<HTMLElement>(".overflow-y-auto"))
      .find((element) => element.textContent?.includes("line 99"));
    expect(scroll).toBeDefined();

    scroll!.scrollTop = 48;
    fireEvent.scroll(scroll!);
    expect(callCount).toBe(1);

    await user.click(screen.getByRole("button", { name: "Load older logs" }));
    await waitFor(() => expect(resolveOlderPage).toBeDefined());
    expect(screen.getByRole("button", { name: "Loading older logs…" })).toBeDisabled();

    await act(async () => resolveOlderPage?.());

    await screen.findByText("older line 0");
    await waitFor(() => {
      expect(mockScrollToOffset).toHaveBeenCalledWith(12048, { align: "start" });
    });
    await user.click(await screen.findByRole("button", { name: "Load older logs" }));
    await waitFor(() => expect(callCount).toBe(3));
  });

  it("continues pagination when searched older logs do not match and the next page is empty", async () => {
    const firstPage = makeLogPage("needle line", 12);
    const nonMatchingPage = makeLogPage("older noise", 11, 50);
    let callCount = 0;
    let resolveNonMatchingPage: (() => void) | undefined;
    let resolveEmptyPage: (() => void) | undefined;

    server.use(
      http.get("/api/v1/deployments/:id/logs", () => {
        callCount++;
        if (callCount === 1) return HttpResponse.json<LogEntry[]>(firstPage);
        if (callCount === 2) {
          return new Promise<Response>((resolve) => {
            resolveNonMatchingPage = () =>
              resolve(HttpResponse.json<LogEntry[]>(nonMatchingPage));
          });
        }
        return new Promise<Response>((resolve) => {
          resolveEmptyPage = () => resolve(HttpResponse.json<LogEntry[]>([]));
        });
      }),
    );

    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Logs" }));
    await screen.findByText("needle line 99");
    await user.type(screen.getByPlaceholderText("Search"), "needle");

    await user.click(screen.getByRole("button", { name: "Load older logs" }));
    await waitFor(() => expect(resolveNonMatchingPage).toBeDefined());
    await act(async () => resolveNonMatchingPage?.());
    expect(screen.queryByText("older noise 0")).not.toBeInTheDocument();

    await user.click(await screen.findByRole("button", { name: "Load older logs" }));
    await waitFor(() => expect(resolveEmptyPage).toBeDefined());
    expect(callCount).toBe(3);
    expect(screen.getByRole("button", { name: "Loading older logs…" })).toBeDisabled();

    await act(async () => resolveEmptyPage?.());
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Load older logs" })).not.toBeInTheDocument();
    });
  });

  it("container switcher appears with multiple containers", async () => {
    setupLogs([]);
    const { user } = renderDeployments(
      makeDeployment({
        workloads: [
          makeWorkload({
            containers: [
              makeContainer({ name: "app" }),
              makeContainer({ name: "sidecar" }),
            ],
          }),
        ],
      }),
    );
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Logs" }));
    expect(await screen.findByText("app")).toBeInTheDocument();
    expect(screen.getByText("sidecar")).toBeInTheDocument();
  });

  it("container switcher is hidden with a single container", async () => {
    setupLogs([]);
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await user.click(screen.getByRole("button", { name: "Logs" }));
    await screen.findByText("No logs in this time window");
    expect(screen.queryByText("Container")).not.toBeInTheDocument();
  });
});

describe("user views deployment history", () => {
  it("shows current deployment tile with commit message and status", async () => {
    renderDeployments();
    expect(await screen.findByText("Fix auth flow")).toBeInTheDocument();
    // Status badge is now driven by useDeploymentStatus; wait for it async.
    expect(await screen.findByText("Active")).toBeInTheDocument();
  });

  it("shows Deploying on history tile when the status endpoint reports deploying", async () => {
    // Status is now server-derived. The handler joins DB status + K8s
    // readiness; from the client's POV we just mock the endpoint and assert
    // the badge mirrors it.
    server.use(
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({ value: "deploying" }),
      ),
    );
    renderDeployments();
    expect(await screen.findByText("Fix auth flow")).toBeInTheDocument();
    expect(await screen.findByText("Deploying")).toBeInTheDocument();
    expect(screen.queryByText("Active")).not.toBeInTheDocument();
  });

  it("View all expands to show full history", async () => {
    server.use(
      http.get("/api/v1/agents/:account/:name/deployment/history", () =>
        HttpResponse.json<DeploymentHistoryResponse>({
          deployments: [
            makeHistoryRecord({ revision: 3, commit_message: "Latest fix" }),
            makeHistoryRecord({
              revision: 2,
              is_current: false,
              commit_message: "Add caching",
              build_id: "bbbb2222",
            }),
            makeHistoryRecord({
              revision: 1,
              is_current: false,
              commit_message: "Initial deploy",
              build_id: "cccc3333",
            }),
          ],
          count: 3,
        }),
      ),
    );
    const { user } = renderDeployments();
    expect(await screen.findByText("Latest fix")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /view all/i }));
    expect(await screen.findByText("Add caching")).toBeInTheDocument();
    expect(screen.getByText("Initial deploy")).toBeInTheDocument();
  });

  it("View all button is hidden when only one deployment exists", async () => {
    renderDeployments();
    await screen.findByText("Fix auth flow");
    expect(screen.queryByRole("button", { name: /view all/i })).not.toBeInTheDocument();
  });

  it("shows ast push label for direct deployments", async () => {
    server.use(
      http.get("/api/v1/agents/:account/:name/deployment/history", () =>
        HttpResponse.json<DeploymentHistoryResponse>({
          deployments: [makeHistoryRecord({ source: "direct", commit_message: undefined })],
          count: 1,
        }),
      ),
    );
    renderDeployments();
    expect(await screen.findByText("ast push")).toBeInTheDocument();
  });

  it("shows branch name for GitHub deployments", async () => {
    renderDeployments();
    expect(await screen.findByText("main")).toBeInTheDocument();
  });
});

describe("user manages deployment lifecycle", () => {
  it("Pause stops the current deployment", async () => {
    const stopHandler = vi.fn();
    server.use(
      http.post("/api/v1/deployments/:id/stop", ({ params }) => {
        stopHandler(params);
        return HttpResponse.json({ status: "ok", deployment_id: params.id });
      }),
    );
    const { user } = renderDeployments();
    await screen.findByText("Fix auth flow");

    await user.click(screen.getByRole("button", { name: /deployment actions/i }));
    await user.click(await screen.findByRole("menuitem", { name: /pause/i }));
    await waitFor(() => expect(stopHandler).toHaveBeenCalledWith({ id: "dep-1" }));
  });

  it("paused deployment shows Resume instead of Pause", async () => {
    const { user } = renderDeployments(
      makeDeployment({ status: "stopped" }),
    );
    await screen.findByText("Fix auth flow");

    await user.click(screen.getByRole("button", { name: /deployment actions/i }));
    expect(await screen.findByRole("menuitem", { name: /resume/i })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: /^pause$/i })).not.toBeInTheDocument();
  });

  it("Restart opens confirmation dialog and confirming restarts the deployment", async () => {
    const restartHandler = vi.fn();
    server.use(
      http.post("/api/v1/deployments/:id/restart", ({ params }) => {
        restartHandler(params);
        return HttpResponse.json({ status: "ok", pods: ["pod-1"] });
      }),
    );
    const { user } = renderDeployments();
    await screen.findByText("Fix auth flow");

    await user.click(screen.getByRole("button", { name: /deployment actions/i }));
    await user.click(await screen.findByRole("menuitem", { name: /restart/i }));

    expect(await screen.findByText("Restart deployment?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Restart" }));
    await waitFor(() => expect(restartHandler).toHaveBeenCalledWith({ id: "dep-1" }));
  });

  it("canceling the Restart dialog does not call the mutation", async () => {
    const restartHandler = vi.fn();
    server.use(
      http.post("/api/v1/deployments/:id/restart", ({ params }) => {
        restartHandler(params);
        return HttpResponse.json({ status: "ok", pods: [] });
      }),
    );
    const { user } = renderDeployments();
    await screen.findByText("Fix auth flow");

    await user.click(screen.getByRole("button", { name: /deployment actions/i }));
    await user.click(await screen.findByRole("menuitem", { name: /restart/i }));
    await user.click(await screen.findByRole("button", { name: /cancel/i }));
    expect(restartHandler).not.toHaveBeenCalled();
  });

  it("Redeploy navigates to the configure page", async () => {
    const { user } = renderDeployments();
    await screen.findByText("Fix auth flow");

    await user.click(screen.getByRole("button", { name: /deployment actions/i }));
    await user.click(await screen.findByRole("menuitem", { name: /redeploy/i }));
    expect(await screen.findByTestId("configure-page")).toBeInTheDocument();
  });

  it("Rollback on historical tile navigates to configure", async () => {
    server.use(
      http.get("/api/v1/agents/:account/:name/deployment/history", () =>
        HttpResponse.json<DeploymentHistoryResponse>({
          deployments: [
            makeHistoryRecord({ revision: 2, commit_message: "Latest" }),
            makeHistoryRecord({
              revision: 1,
              is_current: false,
              commit_message: "Old revision",
              build_id: "old-build",
            }),
          ],
          count: 2,
        }),
      ),
    );
    const { user } = renderDeployments();
    await user.click(await screen.findByRole("button", { name: /view all/i }));
    await screen.findByText("Old revision");

    await user.click(screen.getByRole("button", { name: /revision actions/i }));
    await user.click(await screen.findByRole("menuitem", { name: /rollback/i }));
    expect(await screen.findByTestId("configure-page")).toBeInTheDocument();
  });
});

describe("user sees upgrade nudge", () => {
  it("shows nudge when a newer build is available", async () => {
    server.use(
      http.get("/api/v1/agents/:account", () =>
        HttpResponse.json({
          agents: [
            {
              name: "my-agent",
              account: "testuser",
              registry: "r.example.com",
              visibility: "public",
              versions: [
                {
                  build_id: "newer-build",
                  spec: {},
                  published_at: "2025-05-01T00:00:00Z",
                },
              ],
            },
          ],
          count: 1,
        }),
      ),
    );
    renderDeployments(makeDeployment({
      build_id: "a1b2c3d4",
      latest_build_id: "newer-build",
      source_account: "testuser",
    }));
    expect(await screen.findByText("New build available")).toBeInTheDocument();
  });

  it("does not show nudge when build is already latest", async () => {
    server.use(
      http.get("/api/v1/agents/:account", () =>
        HttpResponse.json({
          agents: [
            {
              name: "my-agent",
              account: "testuser",
              registry: "r.example.com",
              versions: [
                {
                  build_id: "a1b2c3d4",
                  spec: {},
                  published_at: "2025-04-01T00:00:00Z",
                },
              ],
            },
          ],
          count: 1,
        }),
      ),
    );
    renderDeployments(makeDeployment({ build_id: "a1b2c3d4", source_account: "testuser" }));
    await screen.findByText("Fix auth flow");
    expect(screen.queryByText("New build available")).not.toBeInTheDocument();
  });
});

describe("user switches pod detail tabs", () => {
  it("switching to Events shows events and hides General content", async () => {
    server.use(
      http.get("/api/v1/deployments/:id/events", () =>
        HttpResponse.json<DeploymentEventsResponse>({
          events: [
            {
              type: "Normal",
              reason: "Pulled",
              message: "Image pulled",
              object_kind: "Pod",
              object_name: "pod-1",
              count: 1,
              first_timestamp: "2025-04-01T00:00:00Z",
              last_timestamp: "2025-04-01T00:00:00Z",
            },
          ],
        }),
      ),
    );
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await screen.findByText("Domains");

    await user.click(screen.getByRole("button", { name: "Events" }));
    expect(await screen.findByText("Pulled")).toBeInTheDocument();
    expect(screen.queryByText("Domains")).not.toBeInTheDocument();
  });

  it("switching back to General from Logs restores General content", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    await screen.findByText("Domains");

    await user.click(screen.getByRole("button", { name: "Logs" }));
    await waitFor(() => expect(screen.queryByText("Domains")).not.toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "General" }));
    expect(await screen.findByText("Domains")).toBeInTheDocument();
  });
});
