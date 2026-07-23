import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import type { WorkloadDetail } from "@/lib/api";
import { renderWithProviders } from "@/test/test-utils";
import { derivePodStatus, PodTile, PodTileContent } from "./PodTile";

// Regression: Go nil slices serialize as JSON `null`, so the `containers`
// field arrives undefined for Job/CronJob workloads that have no live pods.
// Any access into `workload.containers` without a guard crashes the deployment
// view. These tests pin the kind-aware status mapping and prove that every
// path through PodTile rendering tolerates a missing `containers` array.

function makeWorkload(overrides?: Partial<WorkloadDetail>): WorkloadDetail {
  return {
    name: "ingestion-acme-1234",
    kind: "Job",
    component: "ingestion-acme",
    age: "2m",
    containers: undefined,
    ...overrides,
  };
}

describe("derivePodStatus", () => {
  it("returns Starting for an undefined workload", () => {
    expect(derivePodStatus(undefined)).toEqual({ status: "pending", label: "Starting" });
  });

  it("maps Job statuses to the Job vocab", () => {
    const cases: Array<[string, { status: string; label: string }]> = [
      ["Pending", { status: "pending", label: "Pending" }],
      ["Running", { status: "healthy", label: "Running" }],
      ["Succeeded", { status: "healthy", label: "Completed" }],
      ["Failed", { status: "unhealthy", label: "Failed" }],
    ];
    for (const [status, expected] of cases) {
      expect(derivePodStatus(makeWorkload({ kind: "Job", status }))).toEqual(expected);
    }
  });

  it("maps CronJob statuses to the CronJob vocab", () => {
    const cases: Array<[string, { status: string; label: string }]> = [
      ["Idle", { status: "healthy", label: "Idle" }],
      ["Active", { status: "healthy", label: "Running" }],
      ["Suspended", { status: "warning", label: "Suspended" }],
    ];
    for (const [status, expected] of cases) {
      expect(derivePodStatus(makeWorkload({ kind: "CronJob", status }))).toEqual(expected);
    }
  });

  it("does not crash on Deployment with undefined containers", () => {
    const workload = makeWorkload({ kind: "Deployment", containers: undefined });
    expect(derivePodStatus(workload)).toEqual({ status: "pending", label: "Starting" });
  });

  it("returns Starting for Deployment with empty containers", () => {
    const workload = makeWorkload({ kind: "Deployment", containers: [] });
    expect(derivePodStatus(workload)).toEqual({ status: "pending", label: "Starting" });
  });

  it("reports Error for a capitalized Waiting container (server casing)", () => {
    const workload = makeWorkload({
      kind: "StatefulSet",
      containers: [
        { name: "app", state: "Waiting", ready: false, restart_count: 0, message: "Couldn't pull the container image" },
        { name: "messaging", state: "Running", ready: true, restart_count: 0 },
      ],
    });
    expect(derivePodStatus(workload)).toEqual({ status: "unhealthy", label: "Error" });
  });
});

describe("PodTile rendering — null-containers regression", () => {
  // Failed Job → derivePodStatus returns "unhealthy", which routes through
  // findUnhealthyContainer. Before the fix, that helper called `.find` on
  // null and crashed the whole pod graph.
  it("renders a Failed Job with undefined containers without throwing", () => {
    renderWithProviders(
      <PodTile
        workload={makeWorkload({ kind: "Job", status: "Failed", containers: undefined })}
        deploymentId="dep-1"
      />,
    );
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText("ingestion-acme")).toBeInTheDocument();
  });

  it("renders a Failed Job with an empty containers array", () => {
    renderWithProviders(
      <PodTile
        workload={makeWorkload({ kind: "Job", status: "Failed", containers: [] })}
        deploymentId="dep-1"
      />,
    );
    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("renders a Suspended CronJob with undefined containers (warningMessage path)", () => {
    renderWithProviders(
      <PodTile
        workload={makeWorkload({ kind: "CronJob", status: "Suspended", containers: undefined })}
        deploymentId="dep-1"
      />,
    );
    expect(screen.getByText("Suspended")).toBeInTheDocument();
  });

  it("renders an Idle CronJob with undefined containers", () => {
    renderWithProviders(
      <PodTile
        workload={makeWorkload({ kind: "CronJob", status: "Idle", containers: undefined })}
        deploymentId="dep-1"
      />,
    );
    expect(screen.getByText("Idle")).toBeInTheDocument();
  });
});

describe("PodTileContent log-issue indicator", () => {
  it("shows the error indicator when logIssue is 'error'", () => {
    renderWithProviders(
      <PodTileContent name="agent" status="healthy" logIssue="error" />,
    );
    expect(screen.getByLabelText("Errors found in logs")).toBeInTheDocument();
  });

  it("shows the warning indicator when logIssue is 'warning'", () => {
    renderWithProviders(
      <PodTileContent name="agent" status="healthy" logIssue="warning" />,
    );
    expect(screen.getByLabelText("Warnings found in logs")).toBeInTheDocument();
  });

  it("hides the indicator by default", () => {
    renderWithProviders(<PodTileContent name="agent" status="healthy" />);
    expect(screen.queryByLabelText("Errors found in logs")).toBeNull();
    expect(screen.queryByLabelText("Warnings found in logs")).toBeNull();
  });
});
