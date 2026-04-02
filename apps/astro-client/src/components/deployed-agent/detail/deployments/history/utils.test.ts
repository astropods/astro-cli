import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { AgentDeployment, DeploymentHistoryRecord } from "@/lib/api";
import {
  formatDurationMs,
  resolveDeployedAtMs,
  deploymentHistoryDurationMs,
  deploymentHistoryUiStatus,
  statusVariant,
  statusLabel,
} from "./utils";

// ── helpers ──────────────────────────────────────────────────────────────────

function makeRecord(overrides: Partial<DeploymentHistoryRecord> = {}): DeploymentHistoryRecord {
  return {
    id: "rec-1",
    agent_name: "my-agent",
    revision: 1,
    build_id: "abc12345",
    namespace: "default",
    display_name: "My Agent",
    is_current: false,
    status: "undeployed",
    deployed_at: new Date(1_000_000_000_000).toISOString(), // fixed epoch
    spec: {},
    ...overrides,
  };
}

function makeLive(overrides: Partial<AgentDeployment> = {}): AgentDeployment {
  return {
    id: "dep-1",
    name: "my-agent",
    status: "active",
    namespace: "default",
    ready: 1,
    replicas: 1,
    created_at: new Date(1_000_000_000_000).toISOString(),
    build_id: "abc12345",
    ...overrides,
  } as AgentDeployment;
}

// ── formatDurationMs ──────────────────────────────────────────────────────────

describe("formatDurationMs", () => {
  it("returns — for negative values", () => expect(formatDurationMs(-1)).toBe("—"));
  it("returns — for non-finite values", () => expect(formatDurationMs(NaN)).toBe("—"));
  it("formats seconds when under 60s", () => expect(formatDurationMs(45_000)).toBe("45s"));
  it("rounds to nearest second", () => expect(formatDurationMs(45_499)).toBe("45s"));
  it("formats minutes when under 60m", () => expect(formatDurationMs(5 * 60_000)).toBe("5m"));
  it("formats hours when under 48h", () => expect(formatDurationMs(3 * 3_600_000)).toBe("3h"));
  it("formats days when 48h or more", () => expect(formatDurationMs(48 * 3_600_000)).toBe("2d"));
  it("returns 0s for zero", () => expect(formatDurationMs(0)).toBe("0s"));
});

// ── resolveDeployedAtMs ───────────────────────────────────────────────────────

describe("resolveDeployedAtMs", () => {
  const epoch = 1_000_000_000_000;

  it("returns deployed_at ms for past records", () => {
    const rec = makeRecord({ is_current: false, deployed_at: new Date(epoch).toISOString() });
    expect(resolveDeployedAtMs(rec, makeLive())).toBe(epoch);
  });

  it("returns deployed_at ms for current record with valid date", () => {
    const rec = makeRecord({ is_current: true, deployed_at: new Date(epoch + 5000).toISOString() });
    expect(resolveDeployedAtMs(rec, makeLive({ created_at: new Date(epoch).toISOString() }))).toBe(epoch + 5000);
  });

  it("falls back to live.created_at when current record has invalid deployed_at", () => {
    const rec = makeRecord({ is_current: true, deployed_at: "invalid-date" });
    const live = makeLive({ created_at: new Date(epoch).toISOString() });
    expect(resolveDeployedAtMs(rec, live)).toBe(epoch);
  });
});

// ── deploymentHistoryDurationMs ───────────────────────────────────────────────

describe("deploymentHistoryDurationMs", () => {
  const NOW = 1_700_000_000_000;
  const start = NOW - 10 * 60_000; // 10 min ago

  beforeEach(() => { vi.setSystemTime(NOW); });
  afterEach(() => { vi.useRealTimers(); });

  it("returns elapsed time for current deployment", () => {
    const rec = makeRecord({ is_current: true, deployed_at: new Date(start).toISOString() });
    const live = makeLive({ created_at: new Date(start).toISOString() });
    const dur = deploymentHistoryDurationMs(rec, 0, [rec], live, true);
    expect(dur).toBeCloseTo(10 * 60_000, -2);
  });

  it("returns null for a past record with no successor (idx === 0, not current)", () => {
    const rec = makeRecord({ deployed_at: new Date(start).toISOString() });
    const live = makeLive();
    expect(deploymentHistoryDurationMs(rec, 0, [rec], live, false)).toBeNull();
  });

  it("computes duration between successive past records", () => {
    const older = makeRecord({ deployed_at: new Date(start).toISOString() });
    const newer = makeRecord({ deployed_at: new Date(start + 5 * 60_000).toISOString() });
    const live = makeLive();
    // older is at idx=1, newer is at idx=0 (sorted descending)
    const dur = deploymentHistoryDurationMs(older, 1, [newer, older], live, false);
    expect(dur).toBe(5 * 60_000);
  });

  it("returns null when deployed_at is invalid", () => {
    const rec = makeRecord({ deployed_at: "bad-date" });
    expect(deploymentHistoryDurationMs(rec, 0, [rec], makeLive(), false)).toBeNull();
  });
});

// ── deploymentHistoryUiStatus ─────────────────────────────────────────────────

describe("deploymentHistoryUiStatus", () => {
  it("returns undeployed for non-current records regardless of live status", () => {
    const rec = makeRecord({ is_current: false });
    expect(deploymentHistoryUiStatus(rec, makeLive({ status: "active" }))).toBe("undeployed");
  });

  it("returns active for a live active deployment", () => {
    const rec = makeRecord({ is_current: true });
    expect(deploymentHistoryUiStatus(rec, makeLive({ status: "active" }))).toBe("active");
  });

  it("maps error live status to failed", () => {
    const rec = makeRecord({ is_current: true });
    expect(deploymentHistoryUiStatus(rec, makeLive({ status: "error" }))).toBe("failed");
  });

  it("maps deploying live status to deploying", () => {
    const rec = makeRecord({ is_current: true });
    expect(deploymentHistoryUiStatus(rec, makeLive({ status: "pending" }))).toBe("deploying");
  });

  it("maps inactive live status (replicas 0) to inactive", () => {
    const rec = makeRecord({ is_current: true });
    expect(deploymentHistoryUiStatus(rec, makeLive({ status: "scaled_down", ready: 0, replicas: 0 }))).toBe("inactive");
  });
});

// ── statusVariant ─────────────────────────────────────────────────────────────

describe("statusVariant", () => {
  const cases = [
    ["failed", "error"],
    ["undeployed", "muted"],
    ["inactive", "muted"],
    ["deploying", "warning"],
    ["undeploying", "muted"],
    ["active", "success"],
    ["restarting", "warning"],
    ["pausing", "error"],
    ["resuming", "success"],
  ] as const;

  it.each(cases)("maps %s → %s", (status, expected) => {
    expect(statusVariant(status)).toBe(expected);
  });
});

// ── statusLabel ───────────────────────────────────────────────────────────────

describe("statusLabel", () => {
  const cases = [
    ["active", "Active"],
    ["inactive", "Inactive"],
    ["deploying", "Deploying"],
    ["undeploying", "Undeploying"],
    ["failed", "Failed"],
    ["restarting", "Restarting"],
    ["pausing", "Pausing"],
    ["resuming", "Resuming"],
    ["undeployed", "Undeployed"],
  ] as const;

  it.each(cases)("maps %s → %s", (status, expected) => {
    expect(statusLabel(status)).toBe(expected);
  });
});
