import { describe, expect, it } from "vitest";
import { mapDeploymentStatus } from "./deployment-utils";
import type { AgentDeployment } from "./api";

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
});
