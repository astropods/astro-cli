import { describe, expect, it } from "vitest";
import {
  isLaunchReady,
  isPausedState,
  launchUnavailableMessage,
} from "./deployment-utils";
import type { AgentDeployment } from "./api";

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
