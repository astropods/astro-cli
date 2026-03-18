import type { DeploymentTemplate } from "@/lib/api";
import type { DeployFormInitialValues } from "./useDeployForm";
import { ADAPTER_SECRETS, ADAPTER_CONFIG } from "./useDeployForm";
import { SLACK_CONFIG_KEY, deserializeSlackConfig } from "./slackConfig";

const adapterFieldKeys = new Set(
  [ADAPTER_SECRETS, ADAPTER_CONFIG].flatMap((map) =>
    Object.values(map).flatMap((fields) => fields.map((f) => f.key)),
  ),
);

/** Extract form initial values from a pre-filled deployment template. */
export const extractInitialValues = (template: DeploymentTemplate, account: string): DeployFormInitialValues => {
  const variableValues: Record<string, string> = {};
  const adapterCredentials: Record<string, string> = {};

  if (template.variables) {
    for (const [key, v] of Object.entries(template.variables)) {
      const val = v.value ?? v.default ?? "";
      // SLACK_CONFIG is a compound field — expand into three virtual fields
      if (key === SLACK_CONFIG_KEY) {
        const parsed = deserializeSlackConfig(val);
        for (const [vKey, vVal] of Object.entries(parsed)) {
          adapterCredentials[vKey] = vVal;
        }
        continue;
      }
      const isAdapterField =
        adapterFieldKeys.has(key) ||
        v.targets?.some((t: string) => t.startsWith("interface."));
      if (isAdapterField) {
        adapterCredentials[key] = val;
      } else {
        variableValues[key] = val;
      }
    }
  }

  const interfaces = template.interfaces as Record<string, unknown> | undefined;
  const adapters = interfaces?.adapters;
  const selectedAdapters: string[] = Array.isArray(adapters) ? adapters : ["web"];

  const ingestionSchedules: Record<string, string> = {};
  if (template.ingestion) {
    for (const [name, ing] of Object.entries(template.ingestion)) {
      if (ing.trigger?.type === "schedule") {
        ingestionSchedules[name] = ing.trigger.schedule ?? "";
      }
    }
  }

  return {
    deployName: template.target.display_name || "",
    targetAccount: account,
    variableValues,
    selectedAdapters,
    adapterCredentials,
    ingestionSchedules,
  };
};
