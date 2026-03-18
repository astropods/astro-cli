export const SLACK_CONFIG_KEY = "SLACK_CONFIG";

/**
 * Serializes the three virtual Slack config fields into a SLACK_CONFIG JSON string.
 * Returns "" when all fields are empty (messaging module treats empty as "use defaults").
 */
export function serializeSlackConfig(values: Record<string, string>): string {
  const reactions = values["SLACK_ACTIONABLE_REACTIONS"]?.trim();
  const channels = values["SLACK_ALLOWED_CHANNEL_IDS"]?.trim();
  const users = values["SLACK_ALLOWED_USER_IDS"]?.trim();

  const cfg: Record<string, string[]> = {};
  if (reactions) cfg["actionable_reactions"] = reactions.split(",").map((s) => s.trim()).filter(Boolean);
  if (channels) cfg["allowed_channel_ids"] = channels.split(",").map((s) => s.trim()).filter(Boolean);
  if (users) cfg["allowed_user_ids"] = users.split(",").map((s) => s.trim()).filter(Boolean);
  if (Object.keys(cfg).length === 0) return "";
  return JSON.stringify(cfg);
}

/**
 * Parses a SLACK_CONFIG JSON string back into the three virtual field values.
 * Returns empty strings for any missing fields.
 */
export function deserializeSlackConfig(json: string): Record<string, string> {
  if (!json?.trim()) return {};
  try {
    const cfg = JSON.parse(json) as {
      actionable_reactions?: string[];
      allowed_channel_ids?: string[];
      allowed_user_ids?: string[];
    };
    return {
      SLACK_ACTIONABLE_REACTIONS: (cfg.actionable_reactions ?? []).join(", "),
      SLACK_ALLOWED_CHANNEL_IDS: (cfg.allowed_channel_ids ?? []).join(", "),
      SLACK_ALLOWED_USER_IDS: (cfg.allowed_user_ids ?? []).join(", "),
    };
  } catch {
    return {};
  }
}
