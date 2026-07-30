import { describe, it, expect, vi } from "vitest";
import { screen, fireEvent } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/test-utils";
import type { WorkloadDetail, DeploymentEventsResponse } from "@/lib/api";
import { PodDetailPanel } from "./PodDetailPanel";

vi.mock("@/lib/log-stream", () => ({
  LogStreamProvider: ({ children }: { children: React.ReactNode }) => children,
  useLogStream: () => ({
    lines: [],
    status: "idle" as const,
    error: undefined,
    startStream: vi.fn(),
    stopStream: vi.fn(),
  }),
}));

// These cover the Events tab's rendering of server-humanized events. The
// server annotates "stuck — needs action" events (e.g. FailedScheduling) with
// title + guidance; the tab leads with that while preserving the raw reason and
// full K8s message. Events without server copy render verbatim.

const workload: WorkloadDetail = {
  name: "shipmate-agent",
  kind: "Deployment",
  component: "agent",
  containers: [],
};

function mockEvents(events: DeploymentEventsResponse["events"]) {
  server.use(
    http.get("/api/v1/deployments/:id/events", () =>
      HttpResponse.json<DeploymentEventsResponse>({ events }),
    ),
  );
}

describe("PodDetailPanel — Events tab", () => {
  it("leads with the server title + guidance for a stuck event, keeping the raw reason and full message", async () => {
    const message =
      "0/14 nodes are available: 11 Insufficient memory, 4 Insufficient cpu, 1 node(s) had untolerated taint(s).";
    mockEvents([
      {
        type: "Warning",
        reason: "FailedScheduling",
        message,
        object_kind: "Pod",
        object_name: "shipmate-agent-abc123",
        count: 9,
        first_timestamp: "2026-06-29T00:00:00Z",
        last_timestamp: "2026-06-29T00:05:00Z",
        title: "Action required. Deployment stuck",
        guidance:
          "This agent requests more CPU/memory than any node has available, so it can't be placed. Reduce its resources under Configure → Advanced sizing and redeploy.",
      },
    ]);

    renderWithProviders(
      <PodDetailPanel workload={workload} deploymentId="dep-1" onClose={() => {}} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Events" }));

    // The friendly humanized title is the row summary (never the raw message)…
    const summary = await screen.findByText("Action required. Deployment stuck");
    expect(summary).toBeInTheDocument();
    // …expanding reveals the guidance, raw message, and reason.
    fireEvent.click(summary);
    expect(screen.getByText(/Advanced sizing/)).toBeInTheDocument();
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.getByText(/FailedScheduling/)).toBeInTheDocument();
  });

  it("shows per-container status, failure message, and env together on the General tab", () => {
    const withContainers: WorkloadDetail = {
      ...workload,
      containers: [
        { name: "app", state: "Waiting", ready: false, restart_count: 0, message: "Couldn't pull the container image" },
        { name: "messaging", state: "Running", ready: true, restart_count: 0 },
      ],
      env: { agent: [{ name: "API_KEY", value: "secret", is_secret: true }] },
    };

    renderWithProviders(
      <PodDetailPanel workload={withContainers} deploymentId="dep-1" onClose={() => {}} />,
    );

    expect(screen.getByText("Containers")).toBeInTheDocument();
    expect(screen.getByText("app")).toBeInTheDocument();
    expect(screen.getByText("messaging")).toBeInTheDocument();
    expect(screen.getByText("Couldn't pull the container image")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("API_KEY")).toBeInTheDocument();
  });

  it("renders the raw reason and no guidance for events without server copy", async () => {
    mockEvents([
      {
        type: "Warning",
        reason: "Unhealthy",
        message: "Readiness probe failed: HTTP probe failed with statuscode: 503",
        object_kind: "Pod",
        object_name: "shipmate-agent-abc123",
        count: 3,
        first_timestamp: "2026-06-29T00:00:00Z",
        last_timestamp: "2026-06-29T00:00:30Z",
      },
    ]);

    renderWithProviders(
      <PodDetailPanel workload={workload} deploymentId="dep-1" onClose={() => {}} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Events" }));

    // No humanized title, so the reason is the row summary (not the raw message)…
    const summary = await screen.findByText("Unhealthy");
    expect(summary).toBeInTheDocument();
    // …expanding reveals the raw message and no humanized guidance.
    fireEvent.click(summary);
    expect(screen.getByText(/Readiness probe failed/)).toBeInTheDocument();
    expect(screen.queryByText("Action required. Deployment stuck")).not.toBeInTheDocument();
    expect(screen.queryByText(/Advanced sizing/)).not.toBeInTheDocument();
  });
});

describe("PodDetailPanel — tab precedence", () => {
  it("stays on General when an error log is detected", async () => {
    let releaseErrorLog!: () => void;
    const errorLogGate = new Promise<void>((resolve) => {
      releaseErrorLog = resolve;
    });
    server.use(
      http.get("/api/v1/deployments/:id/logs", async () => {
        await errorLogGate;
        return HttpResponse.json([
          {
            timestamp: "2026-06-29T00:00:00Z",
            level: "error",
            message: "Connection refused",
          },
        ]);
      }),
    );
    const workloadWithContainer: WorkloadDetail = {
      ...workload,
      containers: [
        {
          name: "app",
          state: "Running",
          ready: true,
          restart_count: 0,
        },
      ],
    };

    renderWithProviders(
      <PodDetailPanel
        workload={workloadWithContainer}
        deploymentId="dep-1"
        onClose={() => {}}
      />,
    );

    releaseErrorLog();
    expect(await screen.findByText("Errors in logs")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "General" })).toHaveClass(
      "text-foreground",
    );
  });

  it("uses General when a manual pod change mounts a new workload", () => {
    const nextWorkload: WorkloadDetail = {
      ...workload,
      name: "shipmate-worker",
      component: "worker",
    };
    const { rerender } = renderWithProviders(
      <PodDetailPanel
        workload={workload}
        deploymentId="dep-1"
        onClose={() => {}}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Logs" }));
    expect(screen.getByRole("button", { name: "Logs" })).toHaveClass(
      "text-foreground",
    );

    rerender(
      <PodDetailPanel
        workload={nextWorkload}
        deploymentId="dep-1"
        onClose={() => {}}
      />,
    );

    expect(screen.getByRole("button", { name: "General" })).toHaveClass(
      "text-foreground",
    );
  });
});
