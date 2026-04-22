import type { VariableField } from "@/lib/api";

export const SLACK_CONFIG_KEY = "SLACK_CONFIG";

/**
 * Serializes sub-field form values back into a JSON string for an object variable.
 * Form keys use the convention `parentKey.fieldKey` (e.g. "SLACK_CONFIG.actionable_reactions").
 * Fields with datatype "csv" are split into arrays; others are stored as raw strings.
 * Returns "" when all fields are empty.
 */
export function serializeObjectVariable(
  parentKey: string,
  fields: Record<string, VariableField>,
  values: Record<string, string>,
): string {
  const cfg: Record<string, unknown> = {};
  for (const [fieldKey, fieldDef] of Object.entries(fields)) {
    const raw = values[`${parentKey}.${fieldKey}`]?.trim();
    if (!raw) continue;
    if (fieldDef.datatype === "csv") {
      cfg[fieldKey] = raw.split(",").map((s) => s.trim()).filter(Boolean);
    } else {
      cfg[fieldKey] = raw;
    }
  }
  if (Object.keys(cfg).length === 0) return "";
  return JSON.stringify(cfg);
}

/**
 * Parses a JSON string into per-field form values for an object variable.
 * Form keys use the convention `parentKey.fieldKey`.
 * Array values are joined with ", " for display in text inputs.
 */
export function deserializeObjectVariable(
  parentKey: string,
  fields: Record<string, VariableField>,
  json: string,
): Record<string, string> {
  if (!json?.trim()) return {};
  try {
    const cfg = JSON.parse(json) as Record<string, unknown>;
    const result: Record<string, string> = {};
    for (const fieldKey of Object.keys(fields)) {
      const val = cfg[fieldKey];
      const formKey = `${parentKey}.${fieldKey}`;
      if (Array.isArray(val)) {
        const joined = val.join(", ");
        if (joined) result[formKey] = joined;
      } else if (val != null) {
        result[formKey] = String(val);
      }
    }
    return result;
  } catch {
    return {};
  }
}
