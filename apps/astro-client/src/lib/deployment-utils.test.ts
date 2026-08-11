import { describe, expect, it } from "vitest";
import {
  deriveChatComposerState,
  isBillingSuspendedStatus,
  isChatListEligible,
  isLaunchReady,
  isPausedState,
  launchUnavailableMessage,
  getLaunchDisabledMessage,
  withLatestBuildId,
} from "./deployment-utils";
import type {
  AgentDeployment,
  AgentDeploymentSummary,
  DeploymentRuntime,
  DeploymentStatus,
  DeploymentStatusReason,
  DeploymentStatusValue,
} from "./api";

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

describe("withLatestBuildId", () => {
  it("backfills a missing latest build without replacing an authoritative value", () => {
    expect(withLatestBuildId(baseRecord, "latest")?.latest_build_id).toBe("latest");
    expect(withLatestBuildId(make({ latest_build_id: "detail" }), "summary")?.latest_build_id)
      .toBe("detail");
  });
});

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
  it("recognizes Stopped (server-normalized paused status)", () => {
    expect(isPausedState(make({ status: "Stopped" }))).toBe(true);
  });
  it("recognizes raw stopped enum", () => {
    expect(isPausedState(make({ status: "stopped" }))).toBe(true);
  });
  it("returns false for Running", () => {
    expect(isPausedState(make({ status: "Running" }))).toBe(false);
  });
  it("returns false for empty status", () => {
    expect(isPausedState(make({ status: "" }))).toBe(false);
  });
});

describe("deriveChatComposerState", () => {
  const status = (
    value: DeploymentStatusValue,
    reason: DeploymentStatusReason = "ready",
  ): DeploymentStatus => ({ value, reason, details: "" });
  const runtime = (messaging_reachable: boolean): DeploymentRuntime => ({
    ready: 1,
    replicas: 1,
    messaging_reachable,
  });

  it("is 'unknown' while status has not loaded (not optimistic 'ready')", () => {
    // Regression: returning 'ready' here let the inspector fire agent/config
    // before we knew whether the agent was paused/unreachable.
    expect(deriveChatComposerState(undefined, undefined)).toBe("unknown");
    expect(deriveChatComposerState(null, runtime(true))).toBe("unknown");
  });

  it("is 'ready' when active and messaging is reachable", () => {
    expect(deriveChatComposerState(status("active"), runtime(true))).toBe(
      "ready",
    );
  });

  it("is 'ready' when active and runtime has not loaded yet (optimistic only once status is known)", () => {
    expect(deriveChatComposerState(status("active"), undefined)).toBe("ready");
  });

  it("is 'unreachable' when active but the messaging sidecar isn't reachable", () => {
    expect(deriveChatComposerState(status("active"), runtime(false))).toBe(
      "unreachable",
    );
  });

  it("is 'starting' while deploying", () => {
    expect(
      deriveChatComposerState(status("deploying", "provisioning"), undefined),
    ).toBe("starting");
  });

  it("is 'error' on error", () => {
    expect(deriveChatComposerState(status("error", "failed"), undefined)).toBe(
      "error",
    );
  });

  it("distinguishes paused from stopped on inactive via reason", () => {
    expect(
      deriveChatComposerState(status("inactive", "paused"), undefined),
    ).toBe("paused");
    expect(
      deriveChatComposerState(status("inactive", "undeploying"), undefined),
    ).toBe("stopped");
  });

  // The stopped copy says to start the agent, which billing prevents.
  it("is 'suspended', not 'stopped', when billing stopped the agent", () => {
    expect(
      deriveChatComposerState(status("inactive", "suspended"), undefined),
    ).toBe("suspended");
  });

  it("is 'stopped' while undeploying", () => {
    expect(
      deriveChatComposerState(status("undeploying", "undeploying"), undefined),
    ).toBe("stopped");
  });
});

describe("isBillingSuspendedStatus", () => {
  const status = (
    value: DeploymentStatusValue,
    reason: DeploymentStatusReason,
  ): DeploymentStatus => ({ value, reason, details: "" });

  it("reads the reason code, not the record status", () => {
    expect(isBillingSuspendedStatus(status("inactive", "suspended"))).toBe(true);
  });

  // A suspended deployment's record status is "error", so the two must not be
  // confused in either direction.
  it("is false for a failed deploy and for a user pause", () => {
    expect(isBillingSuspendedStatus(status("error", "failed"))).toBe(false);
    expect(isBillingSuspendedStatus(status("inactive", "paused"))).toBe(false);
  });

  it("is false before the status loads", () => {
    expect(isBillingSuspendedStatus(undefined)).toBe(false);
    expect(isBillingSuspendedStatus(null)).toBe(false);
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
