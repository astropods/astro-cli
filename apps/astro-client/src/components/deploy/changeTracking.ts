// Shared deploy-form change counting helpers.

export type KnowledgeBindingMode = "local" | "shared";
export type KnowledgeBindingModes = Record<string, KnowledgeBindingMode>;

export interface KnowledgeBindingSelection {
  bindings: Record<string, string>;
  modes: KnowledgeBindingModes;
}

const stringChangeCount = (a: string, b: string) => a !== b ? 1 : 0;

export function knowledgeBindingChangeCount(
  a: KnowledgeBindingSelection,
  b: KnowledgeBindingSelection,
): number {
  const allKeys = new Set([
    ...Object.keys(a.bindings),
    ...Object.keys(b.bindings),
    ...Object.keys(a.modes),
    ...Object.keys(b.modes),
  ]);
  let count = 0;
  for (const key of allKeys) {
    const aMode = a.modes[key] ?? (a.bindings[key] ? "shared" : "local");
    const bMode = b.modes[key] ?? (b.bindings[key] ? "shared" : "local");
    if (aMode !== bMode || (a.bindings[key] ?? "") !== (b.bindings[key] ?? "")) {
      count++;
    }
  }
  return count;
}

export interface ProvisioningTrackedState {
  agentCpu: string;
  agentMemory: string;
  agentVolumeMount: string;
  agentStorageSize: string;
}

type ProvisioningField = keyof ProvisioningTrackedState;

const PROVISIONING_FIELDS: ProvisioningField[] = [
  "agentCpu",
  "agentMemory",
  "agentVolumeMount",
  "agentStorageSize",
];

export function provisioningChangeCount(
  initial: ProvisioningTrackedState,
  current: ProvisioningTrackedState,
): number {
  let count = 0;
  for (const key of PROVISIONING_FIELDS) {
    count += stringChangeCount(initial[key], current[key]);
  }
  return count;
}
