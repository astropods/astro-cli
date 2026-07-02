import { describe, expect, it } from "vitest";
import {
  knowledgeBindingChangeCount,
  provisioningChangeCount,
} from "./changeTracking";

describe("deploy change tracking helpers", () => {
  it("counts a knowledge binding mode change as one change", () => {
    expect(knowledgeBindingChangeCount(
      { bindings: {}, modes: { postgres: "local" } },
      {
        bindings: { postgres: "arn:knowledge:acct:pg-store" },
        modes: { postgres: "shared" },
      },
    )).toBe(1);
  });

  it("does not count an unchanged shared knowledge binding", () => {
    const selection = {
      bindings: { postgres: "arn:knowledge:acct:pg-store" },
      modes: { postgres: "shared" as const },
    };

    expect(knowledgeBindingChangeCount(selection, selection)).toBe(0);
  });

  it("falls back to binding presence when a knowledge mode is missing", () => {
    expect(knowledgeBindingChangeCount(
      { bindings: { postgres: "arn:knowledge:acct:pg-store" }, modes: {} },
      {
        bindings: { postgres: "arn:knowledge:acct:pg-store" },
        modes: { postgres: "shared" },
      },
    )).toBe(0);
  });

  it("counts each changed provisioning field", () => {
    expect(provisioningChangeCount(
      {
        agentCpu: "500m",
        agentMemory: "512Mi",
        agentVolumeMount: "/data",
        agentStorageSize: "5Gi",
      },
      {
        agentCpu: "1",
        agentMemory: "512Mi",
        agentVolumeMount: "/data",
        agentStorageSize: "10Gi",
      },
    )).toBe(2);
  });
});
