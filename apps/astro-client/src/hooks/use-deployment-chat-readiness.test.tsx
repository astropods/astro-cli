import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import type {
  DeploymentRuntime,
  DeploymentStatus,
  DeploymentStatusReason,
  DeploymentStatusValue,
} from "@/lib/api";
import { useDeploymentChatReadiness } from "./use-deployment-chat-readiness";

const mockStatus = vi.fn();
const mockRuntime = vi.fn();
vi.mock("@/api/queries/deployments", () => ({
  useDeploymentStatus: () => mockStatus(),
  useDeploymentRuntime: () => mockRuntime(),
}));

const status = (
  value: DeploymentStatusValue,
  reason: DeploymentStatusReason = "ready",
): DeploymentStatus => ({ value, reason, details: "" });
const runtime = (messaging_reachable: boolean): DeploymentRuntime => ({
  ready: 1,
  replicas: 1,
  messaging_reachable,
});

beforeEach(() => {
  mockStatus.mockReset();
  mockRuntime.mockReset();
});

function render() {
  return renderHook(() => useDeploymentChatReadiness("d1")).result.current;
}

describe("useDeploymentChatReadiness", () => {
  it("is not ready while status has not loaded", () => {
    mockStatus.mockReturnValue({ data: undefined });
    mockRuntime.mockReturnValue({ data: { runtime: runtime(true) } });
    expect(render()).toMatchObject({ ready: false, resolved: false });
  });

  it("is not ready while runtime has not settled (avoids firing at a mid-load agent)", () => {
    // status active makes state optimistically "ready", but runtime hasn't loaded
    // and hasn't errored, so `resolved` is false and we must not fetch yet.
    mockStatus.mockReturnValue({ data: status("active") });
    mockRuntime.mockReturnValue({ data: undefined, isError: false });
    expect(render()).toMatchObject({ state: "ready", ready: false });
  });

  it("is ready when active and messaging is reachable", () => {
    mockStatus.mockReturnValue({ data: status("active") });
    mockRuntime.mockReturnValue({ data: { runtime: runtime(true) } });
    expect(render()).toMatchObject({ state: "ready", ready: true });
  });

  it("is not ready when active but the sidecar is unreachable", () => {
    mockStatus.mockReturnValue({ data: status("active") });
    mockRuntime.mockReturnValue({ data: { runtime: runtime(false) } });
    expect(render()).toMatchObject({ state: "unreachable", ready: false });
  });

  it("is not ready for a stopped deployment", () => {
    mockStatus.mockReturnValue({ data: status("inactive", "undeploying") });
    mockRuntime.mockReturnValue({ data: { runtime: runtime(false) } });
    expect(render().ready).toBe(false);
  });

  it("treats a runtime read error as settled (DB-backed, cluster-independent)", () => {
    // runtime errored → no runtime data → state falls back to optimistic "ready"
    // for an active agent, and `resolved` is true, so the gate opens rather than
    // pinning the caller on "Checking…" forever.
    mockStatus.mockReturnValue({ data: status("active") });
    mockRuntime.mockReturnValue({ data: undefined, isError: true });
    expect(render()).toMatchObject({ ready: true });
  });
});
