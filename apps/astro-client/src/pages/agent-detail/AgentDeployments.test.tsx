import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { screen, cleanup, waitFor } from "@testing-library/react";
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
} from "@/lib/api";
import type { LogEntry } from "@/lib/log-utils";
import AgentDeployments from "./AgentDeployments";

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
  useVirtualizer: (opts: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: opts.count }, (_, i) => ({
        key: i,
        index: i,
        start: i * 24,
        size: 24,
      })),
    getTotalSize: () => opts.count * 24,
    measureElement: vi.fn(),
    scrollToIndex: vi.fn(),
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
  );
});

afterEach(cleanup);
afterEach(() => {
  server.resetHandlers();
  mockStartStream.mockClear();
  mockStopStream.mockClear();
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
    env: [
      { name: "PORT", value: "8080" },
      { name: "OPENAI_API_KEY", value: "sk-test-123", from: "secret:openai" },
    ],
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
    ready: 1,
    created_at: "2025-04-01T00:00:00Z",
    components: ["agent"],
    workloads: [makeWorkload()],
    external_urls: [{ name: "http", url: "https://public.example.com", type: "http" }],
    avatar_colors: { accent: "#2dd4bf", base: "#0f766e", vibrant: "#2dd4bf", vibrant_light: "#5eead4", accent_light: "#99f6e4", background: "#042f2e", foreground: "#f0fdfa", glow: "#2dd4bf" },
    ...overrides,
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
function renderDeployments(deployment?: AgentDeployment) {
  const dep = deployment ?? makeDeployment();
  const user = userEvent.setup();

  const result = renderRoute(
    [
      {
        path: "/:account/agents/:deploymentId",
        Component: () => (
          <Outlet
            context={{
              deployment: dep,
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

  it("shows Starting status when containers are empty", async () => {
    renderDeployments(
      makeDeployment({
        workloads: [makeWorkload({ containers: [] })],
      }),
    );
    expect(await screen.findByText("Starting")).toBeInTheDocument();
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

  it("General tab shows Domains and Environment Variables sections", async () => {
    const { user } = renderDeployments();
    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("Domains")).toBeInTheDocument();
    expect(screen.getByText("Environment Variables")).toBeInTheDocument();
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
            containers: [makeContainer({ env: [] })],
          }),
        ],
      }),
    );
    await user.click(await screen.findByText("agent"));
    expect(await screen.findByText("No domains configured")).toBeInTheDocument();
    expect(screen.getByText("No environment variables")).toBeInTheDocument();
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
    expect(screen.getByText("Unhealthy")).toBeInTheDocument();
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
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("shows Deploying on history tile when replica count is ready but a sidecar is not", async () => {
    renderDeployments(
      makeDeployment({
        status: "Running",
        replicas: 1,
        ready: 1,
        workloads: [
          makeWorkload({
            kind: "Deployment",
            name: "my-agent",
            containers: [
              makeContainer({ name: "app", ready: true }),
              makeContainer({ name: "messaging", ready: false, state: "waiting" }),
            ],
          }),
        ],
      }),
    );
    expect(await screen.findByText("Fix auth flow")).toBeInTheDocument();
    expect(screen.getByText("Deploying")).toBeInTheDocument();
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
      makeDeployment({ status: "scaled_down" }),
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
    renderDeployments(makeDeployment({ build_id: "a1b2c3d4", source_account: "testuser" }));
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
