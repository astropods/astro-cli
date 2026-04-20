import type { DeploymentTemplate } from "@/lib/api";
import type { DeployFormInitialValues } from "./useDeployForm";
import { getVariableDefault } from "./VariableField";
import { SLACK_CONFIG_KEY, deserializeSlackConfig } from "./slackConfig";

/**
 * Compute all initial form values synchronously from a deployment template.
 *
 * Used on the fresh-deploy path where no existing deployment values exist.
 * This ensures select fields (and other fields with defaults) are populated
 * on the very first render, without relying on effects.
 */
export function computeFormDefaults(
  template: DeploymentTemplate | null | undefined,
  name: string,
): DeployFormInitialValues {
  const deployName = slugToTitle(name);

  if (!template) {
    return { deployName, selectedAdapters: ["web"] };
  }

  // Variable defaults (agent/ingestion-targeted variables)
  const variableValues: Record<string, string> = {};
  // Adapter credential defaults (interface-targeted variables)
  const adapterCredentials: Record<string, string> = {};

  if (template.variables) {
    for (const [key, v] of Object.entries(template.variables)) {
      const isAgentOrIngestion = v.targets?.some(
        (t) => t === "agent" || t.startsWith("ingestion"),
      );
      if (isAgentOrIngestion) {
        variableValues[key] = v.default ?? getVariableDefault({
          datatype: v.datatype,
        });
        continue;
      }

      const isInterface = v.targets?.some((t) => t.startsWith("interface."));
      if (isInterface && key !== SLACK_CONFIG_KEY && v.default) {
        adapterCredentials[key] = v.default;
      }
    }

    // SLACK_CONFIG compound field — parse into virtual fields
    const slackCfgDefault = template.variables[SLACK_CONFIG_KEY]?.default;
    if (slackCfgDefault) {
      const parsed = deserializeSlackConfig(slackCfgDefault);
      for (const [key, val] of Object.entries(parsed)) {
        if (val && !adapterCredentials[key]) adapterCredentials[key] = val;
      }
    }
  }

  // Ingestion schedule defaults
  const ingestionSchedules: Record<string, string> = {};
  if (template.ingestion) {
    for (const [ingName, ing] of Object.entries(template.ingestion)) {
      if (ing.trigger?.type === "schedule") {
        ingestionSchedules[ingName] = ing.trigger.schedule ?? "";
      }
    }
  }

  // Web auth
  const interfaces = template.interfaces as Record<string, unknown> | undefined;
  const webAuth = (interfaces?.auth as Record<string, unknown> | undefined)?.web as Record<string, unknown> | undefined;
  const webAuthEnabled = webAuth?.type === "oidc";

  return {
    deployName,
    variableValues,
    selectedAdapters: ["web"],
    adapterCredentials,
    ingestionSchedules,
    webAuthEnabled,
  };
}

/** Convert a slug like "code-reviewer" to title case: "Code Reviewer" */
function slugToTitle(slug: string): string {
  return slug
    .split(/[-_]+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}
