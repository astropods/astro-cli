import { describe, it, expect, vi, afterEach } from "vitest";
import { screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { useVirtualizer } from "@tanstack/react-virtual";

vi.mock("@tanstack/react-virtual", () => ({ useVirtualizer: vi.fn() }));

vi.mock("../LogStreamProvider", () => ({
  LogStreamProvider: ({ children }: { children: React.ReactNode }) => children,
  useLogStream: () => ({
    lines: [],
    status: "idle" as const,
    error: null as string | null,
    startStream: vi.fn(),
    stopStream: vi.fn(),
  }),
}));

vi.mocked(useVirtualizer).mockImplementation((opts) => ({
  getVirtualItems: () =>
    Array.from({ length: opts.count }, (_, i) => ({ key: i, index: i, start: i * 28, size: 28 })),
  getTotalSize: () => opts.count * 28,
  measureElement: vi.fn(),
  scrollToIndex: vi.fn(),
}) as unknown as ReturnType<typeof useVirtualizer>);
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/test-utils";
import { LogsTab } from "./LogsTab";
import { LogStreamProvider } from "../LogStreamProvider";
import type { AgentDeployment } from "@/lib/api";

afterEach(cleanup);
afterEach(() => server.resetHandlers());

const mockDeployment: AgentDeployment = {
  id: "dep-1",
  name: "feature-flag-assistant",
  build_id: "abc123",
  namespace: "astro-abc123",
  status: "Running",
  replicas: 1,
  ready: 1,
  created_at: "2025-01-01T00:00:00Z",
  components: ["deployment"],
  workloads: [
    {
      name: "agent-workload",
      kind: "Deployment",
      component: "agent",
      age: "1d",
      containers: [
        { name: "app", state: "running", ready: true, restart_count: 0 },
        { name: "sidecar", state: "running", ready: true, restart_count: 0 },
      ],
    },
  ],
};

function setupLogsHandler(message = "log line 1") {
  server.use(
    http.get("/api/v1/deployments/:id/logs", () => {
      return HttpResponse.json([
        { timestamp: "2026-01-01T00:00:01Z", level: "INFO", message },
        { timestamp: "2026-01-01T00:00:02Z", level: "INFO", message: "log line 2" },
      ]);
    }),
  );
}

function renderTab(props: Partial<React.ComponentProps<typeof LogsTab>> = {}) {
  return renderWithProviders(
    <LogStreamProvider>
      <LogsTab deployment={mockDeployment} isCompact={false} {...props} />
    </LogStreamProvider>,
  );
}

describe("LogsTab — default state", () => {
  it("renders the first container as an active tab", () => {
    setupLogsHandler();
    renderTab();
    expect(screen.getByRole("button", { name: /close agent \/ app/i })).toBeInTheDocument();
  });

  it("does not show the empty state on initial render", () => {
    setupLogsHandler();
    renderTab();
    expect(screen.queryByText("Select a container to view logs")).not.toBeInTheDocument();
  });

  it("fetches and displays logs for the active tab", async () => {
    setupLogsHandler("hello from agent");
    renderTab();
    await waitFor(() => expect(screen.getByText(/hello from agent/)).toBeInTheDocument(), { timeout: 3000 });
  });

  it("shows empty state after all tabs are closed", async () => {
    setupLogsHandler();
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: /close agent \/ app/i }));
    await waitFor(() =>
      expect(screen.getByText("Select a container to view logs")).toBeInTheDocument(),
    );
  });

  it("shows no services message when deployment has no workloads", () => {
    renderTab({ deployment: { ...mockDeployment, workloads: [] } });
    expect(screen.getByText("No services available.")).toBeInTheDocument();
  });
});

describe("LogsTab — refresh button", () => {
  it("renders a refresh button", async () => {
    setupLogsHandler();
    renderTab();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Refresh logs" })).toBeInTheDocument(),
    );
  });

  it("clicking refresh triggers another log fetch", async () => {
    let callCount = 0;
    server.use(
      http.get("/api/v1/deployments/:id/logs", () => {
        callCount++;
        return HttpResponse.json([
          { timestamp: "2026-01-01T00:00:01Z", level: "INFO", message: "log line" },
        ]);
      }),
    );
    renderTab();
    await waitFor(() => expect(callCount).toBeGreaterThanOrEqual(1));
    fireEvent.click(screen.getByRole("button", { name: "Refresh logs" }));
    await waitFor(() => expect(callCount).toBeGreaterThanOrEqual(2));
  });

  it("refresh button is disabled when live tail is active", async () => {
    setupLogsHandler();
    renderTab();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Refresh logs" })).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole("button", { name: /Live tail/i }));
    expect(screen.getByRole("button", { name: "Refresh logs" })).toBeDisabled();
  });
});
