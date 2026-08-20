import { describe, it, expect } from "vitest";
import { screen } from "@testing-library/react";
import type { WorkloadDetail } from "@/lib/api";
import { renderWithProviders } from "@/test/test-utils";
import { derivePodStatus, resolvePodStatus, PodTile, PodTileContent } from "./PodTile";

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

  it("reports Starting, not Error, for a container that is still starting up", () => {
    const workload = makeWorkload({
      kind: "StatefulSet",
      containers: [
        { name: "app", state: "Waiting", ready: false, restart_count: 0, message: "Starting up" },
        { name: "messaging", state: "Running", ready: true, restart_count: 0 },
      ],
    });
    expect(derivePodStatus(workload)).toEqual({ status: "pending", label: "Starting" });
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

const GiB = 1024 ** 3;

describe("PodTileContent storage bar", () => {
  it("renders a used/capacity bar when storage is provided", () => {
    renderWithProviders(
      <PodTileContent name="qdrant" status="healthy" storage={{ usedBytes: 3 * GiB, capacityBytes: 10 * GiB }} />,
    );
    const bar = screen.getByRole("progressbar");
    expect(bar).toHaveAttribute("aria-valuenow", "30");
    expect(screen.getByText("3/10 GB")).toBeInTheDocument();
  });

  it("flags low storage with a yellow warning at ≥80% used", () => {
    renderWithProviders(
      <PodTileContent name="qdrant" status="healthy" storage={{ usedBytes: 8.6 * GiB, capacityBytes: 10 * GiB }} />,
    );
    const warn = screen.getByLabelText(/low storage/i);
    expect(warn).toBeInTheDocument();
    expect(warn.querySelector("svg")?.getAttribute("class")).toContain("text-yellow-400");
  });

  it("keeps the yellow low warning below 100% — red is reserved for full", () => {
    renderWithProviders(
      <PodTileContent name="redis" status="healthy" storage={{ usedBytes: 4.85 * GiB, capacityBytes: 5 * GiB }} />,
    );
    const warn = screen.getByLabelText(/low storage/i);
    expect(warn.querySelector("svg")?.getAttribute("class")).toContain("text-yellow-400");
  });

  it("shows no warning with plenty of headroom", () => {
    renderWithProviders(
      <PodTileContent name="postgres" status="healthy" storage={{ usedBytes: 2 * GiB, capacityBytes: 10 * GiB }} />,
    );
    // The warning aria-labels are the only ones mentioning free space.
    expect(screen.queryByLabelText(/free/i)).toBeNull();
  });

  it("turns the whole bar solid red and reads 'full' at 100%", () => {
    renderWithProviders(
      <PodTileContent name="neo4j" status="healthy" storage={{ usedBytes: 8 * GiB, capacityBytes: 8 * GiB }} />,
    );
    const fill = screen.getByRole("progressbar").querySelector("div") as HTMLElement;
    // Fully revealed and recolored to solid red — the neutral/amber ramp is gone.
    expect(fill.style.clipPath).toBe("inset(0 0% 0 0)");
    expect(fill.style.backgroundImage).toContain("red-400");
    expect(fill.style.backgroundImage).not.toContain("muted-foreground");
    expect(fill.style.backgroundImage).not.toContain("yellow-400");
    const warn = screen.getByLabelText(/storage full/i);
    expect(warn).toBeInTheDocument();
    expect(warn.querySelector("svg")?.getAttribute("class")).toContain("text-red-400");
  });

  it("hides the bar when storage is absent", () => {
    renderWithProviders(<PodTileContent name="qdrant" status="healthy" />);
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("hides the bar when capacity is zero (avoids divide-by-zero)", () => {
    renderWithProviders(
      <PodTileContent name="qdrant" status="healthy" storage={{ usedBytes: 0, capacityBytes: 0 }} />,
    );
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("anchors color to the scale: a fixed gradient revealed by clip-path", () => {
    const read = (usedBytes: number) => {
      const { unmount } = renderWithProviders(
        <PodTileContent name="qdrant" status="healthy" storage={{ usedBytes, capacityBytes: 10 * GiB }} />,
      );
      const fill = screen.getByRole("progressbar").querySelector("div") as HTMLElement;
      const rightInset = parseFloat(fill.style.clipPath.match(/inset\(0 ([\d.]+)% 0 0\)/)![1]);
      const gradient = fill.style.backgroundImage;
      unmount();
      return { rightInset, gradient };
    };
    const low = read(2 * GiB); // 20% used → 80% clipped away
    const high = read(9.7 * GiB); // 97% used → 3% clipped away
    expect(low.rightInset).toBeCloseTo(80, 5);
    expect(high.rightInset).toBeCloseTo(3, 5);
    // More usage reveals more of the same gradient — color isn't swapped per fill.
    expect(high.rightInset).toBeLessThan(low.rightInset);
    expect(low.gradient).toContain("yellow-400");
    expect(low.gradient).toContain("red-400");
  });
});

describe("PodTile storage gating", () => {
  const knowledge = (overrides?: Partial<WorkloadDetail>): WorkloadDetail =>
    makeWorkload({
      name: "knowledge-qdrant",
      kind: "StatefulSet",
      component: "knowledge-qdrant",
      containers: [],
      storage_used_bytes: 3 * GiB,
      storage_capacity_bytes: 10 * GiB,
      ...overrides,
    });

  it("shows the bar on a knowledge tile with storage", () => {
    renderWithProviders(<PodTile workload={knowledge()} deploymentId="dep-1" />);
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });

  it("does not show the bar on a non-knowledge tile even when storage is present", () => {
    renderWithProviders(
      <PodTile
        workload={knowledge({ name: "agent", component: "agent" })}
        deploymentId="dep-1"
      />,
    );
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("hides the bar while the deployment is paused", () => {
    renderWithProviders(<PodTile workload={knowledge()} deploymentId="dep-1" paused />);
    expect(screen.queryByRole("progressbar")).toBeNull();
  });
});

// A suspended agent has zero replicas, so derivePodStatus alone reads
// "Starting". The deployment-level state has to win, and stay distinct from a
// pause the user chose.
describe("resolvePodStatus precedence", () => {
  const scaledToZero = { kind: "Deployment", containers: [] } as unknown as WorkloadDetail;

  it("reports Suspended for a billing-stopped deployment", () => {
    expect(resolvePodStatus(scaledToZero, { suspended: true })).toEqual({
      status: "suspended",
      label: "Suspended",
    });
  });

  it("keeps Suspended when the record also looks paused", () => {
    expect(resolvePodStatus(scaledToZero, { suspended: true, paused: true }).label).toBe("Suspended");
  });

  it("still reports Paused for a user-initiated stop", () => {
    expect(resolvePodStatus(scaledToZero, { paused: true }).label).toBe("Paused");
  });

  it("falls through to Starting when neither applies", () => {
    expect(resolvePodStatus(scaledToZero, {}).label).toBe("Starting");
  });
});
