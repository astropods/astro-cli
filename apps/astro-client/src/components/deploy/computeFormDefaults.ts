import type { DeploymentTemplate, TemplateInterfaces } from "@/lib/api";
import type { DeployFormInitialValues } from "./useDeployForm";
import { getVariableDefault } from "./VariableField";
import { deserializeObjectVariable } from "./slackConfig";

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
  respInterfaces?: TemplateInterfaces,
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
      if (!isInterface) continue;

      // Object variables with fields → expand sub-fields
      if (v.datatype === "object" && v.fields && v.default) {
        const parsed = deserializeObjectVariable(key, v.fields, v.default);
        for (const [fKey, fVal] of Object.entries(parsed)) {
          if (fVal && !adapterCredentials[fKey]) adapterCredentials[fKey] = fVal;
        }
      } else if (v.default) {
        adapterCredentials[key] = v.default;
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

  // Web auth — prefer response-level interfaces, fall back to template
  const auth = respInterfaces?.auth
    ?? (template.interfaces as Record<string, unknown> | undefined)?.auth as TemplateInterfaces['auth'] | undefined;
  const webAuthEnabled = auth?.web?.type === "oidc";

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
