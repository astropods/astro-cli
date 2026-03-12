import { describe, it, expect } from "vitest";
import { renderHook } from "@testing-library/react";
import { useChangeTracking, type TrackedFormState } from "./useChangeTracking";

const base: TrackedFormState = {
  deployName: "My Agent",
  variableValues: { OPENAI_API_KEY: "sk-123" },
  selectedAdapters: ["web"],
  adapterCredentials: {},
};

function track(current: Partial<TrackedFormState>) {
  const { result } = renderHook(() => useChangeTracking(base, { ...base, ...current }));
  return result.current;
}

describe("useChangeTracking", () => {
  it("reports clean state when nothing changed", () => {
    const r = track({});
    expect(r.isDirty).toBe(false);
    expect(r.requiresRedeploy).toBe(false);
    expect(r.cosmeticOnly).toBe(false);
    expect(r.changeCount).toBe(0);
    expect(r.dirtyFields.deployName).toBe(false);
    expect(r.dirtyFields.variableValues).toBe(false);
  });

  it("detects cosmetic-only change (deployName)", () => {
    const r = track({ deployName: "Renamed" });
    expect(r.isDirty).toBe(true);
    expect(r.cosmeticOnly).toBe(true);
    expect(r.requiresRedeploy).toBe(false);
    expect(r.changeCount).toBe(1);
    expect(r.dirtyFields.deployName).toBe(true);
  });

  it("detects redeploy-required change (variableValues)", () => {
    const r = track({ variableValues: { OPENAI_API_KEY: "sk-new" } });
    expect(r.isDirty).toBe(true);
    expect(r.requiresRedeploy).toBe(true);
    expect(r.cosmeticOnly).toBe(false);
    expect(r.changeCount).toBe(1);
    expect(r.dirtyFields.variableValues).toBe(true);
  });

  it("detects redeploy-required change (selectedAdapters)", () => {
    const r = track({ selectedAdapters: ["web", "slack"] });
    expect(r.isDirty).toBe(true);
    expect(r.requiresRedeploy).toBe(true);
    expect(r.dirtyFields.selectedAdapters).toBe(true);
  });

  it("detects redeploy-required change (adapterCredentials)", () => {
    const r = track({ adapterCredentials: { SLACK_BOT_TOKEN: "xoxb-new" } });
    expect(r.isDirty).toBe(true);
    expect(r.requiresRedeploy).toBe(true);
    expect(r.dirtyFields.adapterCredentials).toBe(true);
  });

  it("counts multiple variable changes individually", () => {
    const r = track({
      variableValues: { OPENAI_API_KEY: "sk-new", EXTRA: "val" },
    });
    // OPENAI_API_KEY changed + EXTRA added = 2 changes
    expect(r.changeCount).toBe(2);
  });

  it("counts cosmetic + redeploy changes together", () => {
    const r = track({
      deployName: "New Name",
      variableValues: { OPENAI_API_KEY: "sk-new" },
    });
    expect(r.isDirty).toBe(true);
    expect(r.requiresRedeploy).toBe(true);
    expect(r.cosmeticOnly).toBe(false);
    // 1 deploy name + 1 variable = 2
    expect(r.changeCount).toBe(2);
  });

  it("handles added and removed keys in records", () => {
    const r = track({ variableValues: { NEW_KEY: "val" } });
    // OPENAI_API_KEY removed + NEW_KEY added = 2
    expect(r.changeCount).toBe(2);
    expect(r.requiresRedeploy).toBe(true);
  });

  // Adapter order is significant — deployment config is positional, so
  // ["slack", "web"] !== ["web", "slack"] and must trigger a redeploy.
  it("treats adapter order change as dirty (order is significant)", () => {
    const baseWithTwo: TrackedFormState = { ...base, selectedAdapters: ["web", "slack"] };
    const { result } = renderHook(() =>
      useChangeTracking(baseWithTwo, { ...baseWithTwo, selectedAdapters: ["slack", "web"] }),
    );
    expect(result.current.isDirty).toBe(true);
    expect(result.current.requiresRedeploy).toBe(true);
  });

  it("detects disjoint keys with same record length as changed", () => {
    const r = track({ variableValues: { COMPLETELY_NEW: "val" } });
    // original OPENAI_API_KEY removed + COMPLETELY_NEW added = 2 changes
    expect(r.isDirty).toBe(true);
    expect(r.requiresRedeploy).toBe(true);
    expect(r.changeCount).toBe(2);
  });

  it("counts per-key credential changes", () => {
    const baseWithCreds: TrackedFormState = {
      ...base,
      adapterCredentials: { SLACK_BOT_TOKEN: "xoxb-old", SLACK_APP_TOKEN: "xapp-old" },
    };
    const { result } = renderHook(() =>
      useChangeTracking(baseWithCreds, {
        ...baseWithCreds,
        adapterCredentials: { SLACK_BOT_TOKEN: "xoxb-new", SLACK_APP_TOKEN: "xapp-old" },
      }),
    );
    expect(result.current.changeCount).toBe(1);
    expect(result.current.dirtyFields.adapterCredentials).toBe(true);
  });
});
