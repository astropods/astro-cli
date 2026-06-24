import { describe, expect, it } from "vitest";
import {
  isChatEligible,
  isChatListEligible,
  isLaunchReady,
  isPausedState,
  launchUnavailableMessage,
  getLaunchDisabledMessage,
} from "./deployment-utils";
import type { AgentDeployment, AgentDeploymentSummary } from "./api";

// Most of the status-derivation logic that used to live in this module (the
// fat mapDeploymentStatus / hasContainerMismatch helpers) moved server-side
// to GET /deployments/:id/status. The tests for that flow are integration
// tests on the server handler — this file only covers the surviving record-
// level predicates.

const baseRecord: AgentDeployment = {
  id: "dep-1",
  name: "agent",
  build_id: "build-1",
  namespace: "astro-x-0",
  status: "Running",
  replicas: 1,
  created_at: "2026-01-01T00:00:00Z",
  components: [],
};

function make(overrides: Partial<AgentDeployment>): AgentDeployment {
  return { ...baseRecord, ...overrides };
}

const baseSummary: AgentDeploymentSummary = {
  id: "dep-1",
  name: "agent",
  build_id: "b1",
  created_at: "2026-01-01T00:00:00Z",
};

describe("isChatListEligible", () => {
  it("requires messaging_web_configured", () => {
    expect(isChatListEligible({ ...baseSummary })).toBe(false);
    expect(
      isChatListEligible({ ...baseSummary, messaging_web_configured: true }),
    ).toBe(true);
  });
});

describe("isChatEligible", () => {
  it("requires active status when provided", () => {
    const summary = { ...baseSummary, messaging_web_configured: true };
    expect(isChatEligible(summary, "deploying")).toBe(false);
    expect(isChatEligible(summary, "active")).toBe(true);
  });
});

describe("isLaunchReady", () => {
  it("returns false when no messaging endpoint exists", () => {
    expect(isLaunchReady(make({ external_urls: [] }))).toBe(false);
  });

  it("returns false when messaging endpoint exists but ready is omitted", () => {
    const dep = make({
      messaging_configured: true,
      external_urls: [{
        name: "messaging",
        url: "https://agent.example.com",
        type: "messaging",
        message: launchUnavailableMessage,
      }],
    });
    expect(isLaunchReady(dep)).toBe(false);
  });

  it("returns false when ready: false explicitly", () => {
    const dep = make({
      messaging_configured: true,
      external_urls: [{
        name: "messaging",
        url: "https://agent.example.com",
        type: "messaging",
        ready: false,
      }],
    });
    expect(isLaunchReady(dep)).toBe(false);
  });

  it("returns true when messaging endpoint is configured and ready", () => {
    const dep = make({
      messaging_configured: true,
      external_urls: [{
        name: "messaging",
        url: "https://agent.example.com",
        type: "messaging",
        ready: true,
      }],
    });
    expect(isLaunchReady(dep)).toBe(true);
  });

  it("returns false when messaging is not configured in the spec", () => {
    const dep = make({
      messaging_configured: false,
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

describe("isPausedState", () => {
  it("recognizes Stopped (server-normalized for both scaled_down and stopped)", () => {
    expect(isPausedState(make({ status: "Stopped" }))).toBe(true);
  });
  it("recognizes raw scaled_down enum (admingrpc / older callers)", () => {
    expect(isPausedState(make({ status: "scaled_down" }))).toBe(true);
  });
  it("returns false for Running", () => {
    expect(isPausedState(make({ status: "Running" }))).toBe(false);
  });
  it("returns false for empty status", () => {
    expect(isPausedState(make({ status: "" }))).toBe(false);
  });
});

describe("getLaunchDisabledMessage", () => {
  it("returns deploying message for 'deploying' status", () => {
    expect(getLaunchDisabledMessage("deploying")).toBe(
      "Agent is still deploying. Launch will be available once deployment is complete.",
    );
  });

  it("returns deploying message for 'pending' status", () => {
    expect(getLaunchDisabledMessage("pending")).toBe(
      "Agent is still deploying. Launch will be available once deployment is complete.",
    );
  });

  it("returns undeploying message for 'undeploying' status", () => {
    expect(getLaunchDisabledMessage("undeploying")).toBe(
      "Agent is being undeployed. Launch is temporarily unavailable.",
    );
  });

  it("returns error message for 'error' status", () => {
    expect(getLaunchDisabledMessage("error")).toBe(
      "Agent is in an error state. Please check the deployment status.",
    );
  });

  it("returns paused message for 'Stopped' status", () => {
    expect(getLaunchDisabledMessage("Stopped")).toBe(
      "Agent is paused. Resume the agent to launch.",
    );
  });

  it("returns paused message for 'inactive' status", () => {
    expect(getLaunchDisabledMessage("inactive")).toBe(
      "Agent is paused. Resume the agent to launch.",
    );
  });

  it("returns default message for unknown status", () => {
    expect(getLaunchDisabledMessage("unknown")).toBe(
      "Agent is not ready. Launch will be available once the agent is active.",
    );
  });

  it("returns default message for undefined status", () => {
    expect(getLaunchDisabledMessage(undefined)).toBe(
      "Agent is not ready. Launch will be available once the agent is active.",
    );
  });
});
