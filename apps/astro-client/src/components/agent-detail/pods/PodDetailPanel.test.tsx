import { describe, it, expect } from "vitest";
import { screen, fireEvent } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/test-utils";
import type { WorkloadDetail, DeploymentEventsResponse } from "@/lib/api";
import { PodDetailPanel } from "./PodDetailPanel";

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
        title: "Deployment stuck — needs action",
        guidance:
          "This agent requests more CPU/memory than any node has available, so it can't be placed. Reduce its resources under Configure → Advanced sizing and redeploy.",
      },
    ]);

    renderWithProviders(
      <PodDetailPanel workload={workload} deploymentId="dep-1" onClose={() => {}} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Events" }));

    // Server title leads as the headline…
    expect(await screen.findByText("Deployment stuck — needs action")).toBeInTheDocument();
    // …the raw reason is still shown (as a secondary tag)…
    expect(screen.getByText(/FailedScheduling/)).toBeInTheDocument();
    // …the server guidance is rendered…
    expect(screen.getByText(/Advanced sizing/)).toBeInTheDocument();
    // …and the full K8s message is rendered verbatim (not summarized away).
    expect(screen.getByText(message)).toBeInTheDocument();
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

    // Unmapped reason is the headline, verbatim…
    expect(await screen.findByText("Unhealthy")).toBeInTheDocument();
    // …and no humanized guidance is shown.
    expect(screen.queryByText("Deployment stuck — needs action")).not.toBeInTheDocument();
    expect(screen.queryByText(/Advanced sizing/)).not.toBeInTheDocument();
  });
});
