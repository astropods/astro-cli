import type { DeploymentTemplate } from "@/lib/api";
import type { DeployFormInitialValues } from "./useDeployForm";
import { ADAPTER_CREDENTIALS } from "./useDeployForm";

/** Extract form initial values from a pre-filled deployment template. */
export function extractInitialValues(template: DeploymentTemplate, account: string): DeployFormInitialValues {
  const variableValues: Record<string, string> = {};
  const adapterCredentials: Record<string, string> = {};

  const adapterCredKeys = new Set(
    Object.values(ADAPTER_CREDENTIALS).flatMap((creds) => creds.map((c) => c.key)),
  );

  if (template.variables) {
    for (const [key, v] of Object.entries(template.variables)) {
      const val = v.value ?? v.default ?? "";
      const isAdapterCred =
        adapterCredKeys.has(key) ||
        v.targets?.some((t: string) => t.startsWith("interface."));
      if (isAdapterCred) {
        adapterCredentials[key] = val;
      } else {
        variableValues[key] = val;
      }
    }
  }

  const interfaces = template.interfaces as Record<string, unknown> | undefined;
  const adapters = interfaces?.adapters;
  const selectedAdapters: string[] = Array.isArray(adapters) ? adapters : ["web"];

  return {
    deployName: template.target.display_name || "",
    targetAccount: account,
    variableValues,
    selectedAdapters,
    adapterCredentials,
  };
}
