import { describe, expect, it } from "vitest";
import {
  hasContainerMismatch,
  isLaunchReady,
  launchUnavailableMessage,
  mapDeploymentStatus,
} from "./deployment-utils";
import type { AgentDeployment, WorkloadDetail } from "./api";

const baseDeployment: AgentDeployment = {
  id: "dep-1",
  name: "agent",
  build_id: "build-1",
  namespace: "astro-x-0",
  status: "Running",
  replicas: 1,
  ready: 1,
  created_at: "2026-01-01T00:00:00Z",
  components: [],
};

const make = (overrides: Partial<AgentDeployment>): AgentDeployment => ({
  ...baseDeployment,
  ...overrides,
});

function makeWorkload(overrides: Partial<WorkloadDetail> = {}): WorkloadDetail {
  return {
    name: "wl",
    kind: "Deployment",
    component: "agent",
    age: "1h",
    containers: [],
    ...overrides,
  };
}

describe("mapDeploymentStatus", () => {
  it("returns active when ready === replicas and no error", () => {
    expect(mapDeploymentStatus(make({ status: "Running", replicas: 1, ready: 1 }))).toBe("active");
  });

  it("returns deploying when ready < replicas and status is benign", () => {
    expect(mapDeploymentStatus(make({ status: "Running", replicas: 2, ready: 1 }))).toBe("deploying");
  });

  // The 35-minute "stuck Deploying" regression: a failed deployment never
  // reaches ready === replicas, so without this precedence the badge stays on
  // Deploying until an operator notices. The server now ships status=error +
  // error_message; this test pins that the UI honors that precedence.
  it("returns error when status=error wins over ready < replicas", () => {
    expect(
      mapDeploymentStatus(make({ status: "error", replicas: 1, ready: 0, error_message: "ImagePullBackOff" })),
    ).toBe("error");
  });

  it("returns error when status=failed wins over ready < replicas", () => {
    expect(mapDeploymentStatus(make({ status: "failed", replicas: 1, ready: 0 }))).toBe("error");
  });

  it("returns error when status=crashloopbackoff regardless of replicas", () => {
    expect(mapDeploymentStatus(make({ status: "crashloopbackoff", replicas: 1, ready: 1 }))).toBe("error");
  });

  it("returns deploying for status=pending/provisioning", () => {
    expect(mapDeploymentStatus(make({ status: "pending" }))).toBe("deploying");
    expect(mapDeploymentStatus(make({ status: "provisioning" }))).toBe("deploying");
  });

  it("returns undeploying when status=undeploying even with ready replicas", () => {
    expect(mapDeploymentStatus(make({ status: "undeploying", replicas: 1, ready: 1 }))).toBe("undeploying");
  });

  it("returns inactive when replicas=0", () => {
    expect(mapDeploymentStatus(make({ replicas: 0, ready: 0 }))).toBe("inactive");
  });

  it("returns deploying when ready=0 and replicas>0 with benign status", () => {
    expect(mapDeploymentStatus(make({ status: "Running", replicas: 1, ready: 0 }))).toBe("deploying");
  });

  it("returns deploying when ready === replicas but a container is not ready", () => {
    expect(
      mapDeploymentStatus(
        make({
          status: "Running",
          replicas: 1,
          ready: 1,
          workloads: [
            makeWorkload({
              name: "my-agent",
              containers: [
                { name: "app", state: "running", ready: true, restart_count: 0 },
                { name: "messaging", state: "waiting", ready: false, restart_count: 0 },
              ],
            }),
          ],
        }),
      ),
    ).toBe("deploying");
  });

  it("returns active when all Deployment containers are ready", () => {
    expect(
      mapDeploymentStatus(
        make({
          status: "Running",
          replicas: 1,
          ready: 1,
          workloads: [
            makeWorkload({
              name: "my-agent",
              containers: [
                { name: "app", state: "running", ready: true, restart_count: 0 },
                { name: "messaging", state: "running", ready: true, restart_count: 0 },
              ],
            }),
          ],
        }),
      ),
    ).toBe("active");
  });
});

// Regression: server-side `containersFromSpecWithEnv` (deploy.go) seeds Job and
// CronJob containers with `Ready` zero-valued. Without `omitempty` on the JSON
// tag, that lands on the client as `ready: false`. Idle CronJobs and finished
// Jobs therefore look like a readiness mismatch to `hasContainerMismatch` —
// before the kind-gate, that pinned `useDeployment` to a permanent 3s refetch
// for any deployment with ingestion. Job/CronJob health lives on `wl.status`,
// not container readiness, so the helper must ignore those kinds entirely.
describe("hasContainerMismatch", () => {
  it("returns false for null/undefined deployments", () => {
    expect(hasContainerMismatch(null)).toBe(false);
    expect(hasContainerMismatch(undefined)).toBe(false);
  });

  it("flags non-ready containers on Deployment workloads when replicas > 0", () => {
    const dep = make({
      workloads: [
        makeWorkload({ kind: "Deployment", containers: [{ name: "app", state: "waiting", ready: false, restart_count: 0 }] }),
      ],
    });
    expect(hasContainerMismatch(dep)).toBe(true);
  });

  it("flags ready containers when replicas === 0 (paused but not yet drained)", () => {
    const dep = make({
      replicas: 0,
      workloads: [
        makeWorkload({ kind: "Deployment", containers: [{ name: "app", state: "running", ready: true, restart_count: 0 }] }),
      ],
    });
    expect(hasContainerMismatch(dep)).toBe(true);
  });

  it("returns false when a Deployment is fully ready", () => {
    const dep = make({
      workloads: [
        makeWorkload({ kind: "Deployment", containers: [{ name: "app", state: "running", ready: true, restart_count: 0 }] }),
      ],
    });
    expect(hasContainerMismatch(dep)).toBe(false);
  });

  it("ignores Job workloads with spec-seeded ready: false (regression)", () => {
    const dep = make({
      workloads: [
        makeWorkload({ kind: "Deployment", containers: [{ name: "app", state: "running", ready: true, restart_count: 0 }] }),
        makeWorkload({ name: "startup-1", kind: "Job", status: "Succeeded", containers: [{ name: "init", state: "", ready: false, restart_count: 0 }] }),
      ],
    });
    expect(hasContainerMismatch(dep)).toBe(false);
  });

  it("ignores CronJob workloads with spec-seeded ready: false (regression)", () => {
    const dep = make({
      workloads: [
        makeWorkload({ kind: "Deployment", containers: [{ name: "app", state: "running", ready: true, restart_count: 0 }] }),
        makeWorkload({ name: "ingest-acme", kind: "CronJob", status: "Idle", containers: [{ name: "ingest", state: "", ready: false, restart_count: 0 }] }),
      ],
    });
    expect(hasContainerMismatch(dep)).toBe(false);
  });

  it("still flags a real Deployment mismatch even when an ingestion CronJob is present", () => {
    const dep = make({
      workloads: [
        makeWorkload({ kind: "Deployment", containers: [{ name: "app", state: "waiting", ready: false, restart_count: 0 }] }),
        makeWorkload({ name: "ingest-acme", kind: "CronJob", status: "Idle", containers: [{ name: "ingest", state: "", ready: false, restart_count: 0 }] }),
      ],
    });
    expect(hasContainerMismatch(dep)).toBe(true);
  });

  it("also covers StatefulSet workloads", () => {
    const dep = make({
      workloads: [
        makeWorkload({ kind: "StatefulSet", containers: [{ name: "redis", state: "waiting", ready: false, restart_count: 0 }] }),
      ],
    });
    expect(hasContainerMismatch(dep)).toBe(true);
  });
});

describe("isLaunchReady", () => {
  it("returns false when messaging endpoint is not ready", () => {
    const dep = make({
      messaging_available: true,
      external_urls: [{
        name: "messaging",
        url: "https://agent.example.com",
        type: "messaging",
        ready: false,
        message: launchUnavailableMessage,
      }],
    });
    expect(isLaunchReady(dep)).toBe(false);
  });

  it("returns false when ready is omitted (matches API omitempty on false)", () => {
    const dep = make({
      messaging_available: true,
      external_urls: [{
        name: "messaging",
        url: "https://agent.example.com",
        type: "messaging",
        message: launchUnavailableMessage,
      }],
    });
    expect(isLaunchReady(dep)).toBe(false);
  });

  it("returns true when messaging endpoint is ready", () => {
    const dep = make({
      messaging_available: true,
      external_urls: [{
        name: "messaging",
        url: "https://agent.example.com",
        type: "messaging",
        ready: true,
      }],
    });
    expect(isLaunchReady(dep)).toBe(true);
  });

  it("returns false when messaging is unavailable", () => {
    const dep = make({
      messaging_available: false,
      external_urls: [{
        name: "messaging",
        url: "https://agent.example.com",
        type: "messaging",
        ready: true,
      }],
    });
    expect(isLaunchReady(dep)).toBe(false);
  });
});
