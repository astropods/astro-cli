import { describe, it, expect } from "vitest";
import { computeFormDefaults } from "./computeFormDefaults";
import type { DeploymentTemplate } from "@/lib/api";
import { mockTemplate } from "@/test/msw/handlers";

function makeTemplate(overrides: Partial<DeploymentTemplate> = {}): DeploymentTemplate {
  return { ...mockTemplate, ...overrides };
}

const slackConfigFields = {
  actionable_reactions: { label: "Actionable Reactions", datatype: "csv", optional: true },
  allowed_channel_ids: { label: "Allowed Channel IDs", datatype: "csv", optional: true },
  allowed_user_ids: { label: "Allowed User IDs", datatype: "csv", optional: true },
};

describe("computeFormDefaults", () => {
  // --- Null / missing template ---

  it("returns sensible defaults when template is null", () => {
    const result = computeFormDefaults(null, "my-agent");
    expect(result.deployName).toBe("My Agent");
    expect(result.selectedAdapters).toEqual(["web"]);
    expect(result.variableValues).toBeUndefined();
    expect(result.adapterCredentials).toBeUndefined();
    expect(result.ingestionSchedules).toBeUndefined();
    expect(result.webAuthEnabled).toBeUndefined();
  });

  it("returns sensible defaults when template is undefined", () => {
    const result = computeFormDefaults(undefined, "code-reviewer");
    expect(result.deployName).toBe("Code Reviewer");
    expect(result.selectedAdapters).toEqual(["web"]);
  });

  // --- deployName from slug ---

  it("converts hyphenated name to title case", () => {
    const result = computeFormDefaults(null, "code-reviewer");
    expect(result.deployName).toBe("Code Reviewer");
  });

  it("converts underscored name to title case", () => {
    const result = computeFormDefaults(null, "slack_bot");
    expect(result.deployName).toBe("Slack Bot");
  });

  it("handles single word name", () => {
    const result = computeFormDefaults(null, "agent");
    expect(result.deployName).toBe("Agent");
  });

  // --- Select field defaults (the original bug) ---

  it("pre-selects default value for select fields", () => {
    const tpl = makeTemplate({
      variables: {
        ENVIRONMENT: {
          default: "production",
          targets: ["agent"],
          datatype: "string",
          "display-as": "select",
          options: ["production", "staging", "development"],
        },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({ ENVIRONMENT: "production" });
  });

  it("pre-selects default for multiple select fields", () => {
    const tpl = makeTemplate({
      variables: {
        ENVIRONMENT: {
          default: "staging",
          targets: ["agent"],
          "display-as": "select",
          options: ["production", "staging"],
        },
        LOG_LEVEL: {
          default: "info",
          targets: ["agent"],
          "display-as": "select",
          options: ["debug", "info", "warn", "error"],
        },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({
      ENVIRONMENT: "staging",
      LOG_LEVEL: "info",
    });
  });

  // --- Agent/ingestion variable defaults ---

  it("uses v.default for agent-targeted variables", () => {
    const tpl = makeTemplate({
      variables: {
        MODEL_NAME: { default: "gpt-4", targets: ["agent"] },
        TEMPERATURE: { default: "0.7", targets: ["agent"] },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({
      MODEL_NAME: "gpt-4",
      TEMPERATURE: "0.7",
    });
  });

  it("uses v.default for ingestion-targeted variables", () => {
    const tpl = makeTemplate({
      variables: {
        BATCH_SIZE: { default: "100", targets: ["ingestion"] },
        INGEST_MODE: { default: "incremental", targets: ["ingestion.daily_sync"] },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({
      BATCH_SIZE: "100",
      INGEST_MODE: "incremental",
    });
  });

  it("returns empty string for variables without a default", () => {
    const tpl = makeTemplate({
      variables: {
        API_KEY: { targets: ["agent"], secret: true },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({ API_KEY: "" });
  });

  it("returns 'false' for boolean variables without a default", () => {
    const tpl = makeTemplate({
      variables: {
        DEBUG_MODE: { targets: ["agent"], datatype: "boolean" },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({ DEBUG_MODE: "false" });
  });

  it("respects explicit boolean default of 'true'", () => {
    const tpl = makeTemplate({
      variables: {
        VERBOSE: { default: "true", targets: ["agent"], datatype: "boolean" },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({ VERBOSE: "true" });
  });

  // --- Mixed variable targets ---

  it("routes agent vars to variableValues and interface vars to adapterCredentials", () => {
    const tpl = makeTemplate({
      variables: {
        MODEL: { default: "gpt-4", targets: ["agent"] },
        SLACK_BOT_TOKEN: { targets: ["interface.slack"], secret: true },
        CUSTOM_ADAPTER_KEY: { default: "abc", targets: ["interface.custom"] },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({ MODEL: "gpt-4" });
    // Interface vars only included if they have a default
    expect(result.adapterCredentials).toEqual({ CUSTOM_ADAPTER_KEY: "abc" });
  });

  it("routes dual-target variable (agent + interface) to variableValues only", () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_BOT_TOKEN: { default: "xoxb-default", targets: ["agent", "interface.slack"], secret: true },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({ SLACK_BOT_TOKEN: "xoxb-default" });
    expect(result.adapterCredentials).toEqual({});
  });

  it("ignores variables with unrecognized targets (e.g. model)", () => {
    const tpl = makeTemplate({
      variables: {
        MODEL_PARAM: { default: "value", targets: ["model"] },
        AGENT_VAR: { default: "yes", targets: ["agent"] },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({ AGENT_VAR: "yes" });
    expect(result.adapterCredentials).toEqual({});
  });

  it("does not include interface variables without defaults in adapterCredentials", () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_BOT_TOKEN: { targets: ["interface.slack"], secret: true },
        SLACK_APP_TOKEN: { targets: ["interface.slack"], secret: true },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.adapterCredentials).toEqual({});
  });

  // --- SLACK_CONFIG compound field ---

  it("deserializes SLACK_CONFIG default into sub-field credentials", () => {
    const slackCfg = JSON.stringify({
      actionable_reactions: ["ticket", "bug"],
      allowed_channel_ids: ["C12345"],
      allowed_user_ids: [],
    });
    const tpl = makeTemplate({
      variables: {
        SLACK_CONFIG: { default: slackCfg, targets: ["interface.slack"], optional: true, datatype: "object", fields: slackConfigFields },
        SLACK_BOT_TOKEN: { targets: ["interface.slack"], secret: true },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.adapterCredentials?.["SLACK_CONFIG.actionable_reactions"]).toBe("ticket, bug");
    expect(result.adapterCredentials?.["SLACK_CONFIG.allowed_channel_ids"]).toBe("C12345");
    // Empty arrays produce empty string, which is falsy → not included
    expect(result.adapterCredentials?.["SLACK_CONFIG.allowed_user_ids"]).toBeUndefined();
  });

  it("handles SLACK_CONFIG with empty/invalid JSON gracefully", () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_CONFIG: { default: "", targets: ["interface.slack"], optional: true, datatype: "object", fields: slackConfigFields },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.adapterCredentials).toEqual({});
  });

  it("does not put SLACK_CONFIG key itself into adapterCredentials", () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_CONFIG: {
          default: JSON.stringify({ actionable_reactions: ["ticket"] }),
          targets: ["interface.slack"],
          optional: true,
          datatype: "object",
          fields: slackConfigFields,
        },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.adapterCredentials?.SLACK_CONFIG).toBeUndefined();
    expect(result.adapterCredentials?.["SLACK_CONFIG.actionable_reactions"]).toBe("ticket");
  });

  // --- Ingestion schedules ---

  it("extracts schedule triggers from ingestion config", () => {
    const tpl = makeTemplate({
      variables: {},
      ingestion: {
        daily_sync: {
          image: "sync:latest",
          trigger: { type: "schedule", schedule: "0 0 * * *" },
        },
        on_demand: {
          image: "import:latest",
          trigger: { type: "manual" },
        },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.ingestionSchedules).toEqual({ daily_sync: "0 0 * * *" });
  });

  it("returns empty string for schedule trigger without a cron expression", () => {
    const tpl = makeTemplate({
      variables: {},
      ingestion: {
        daily_sync: {
          image: "sync:latest",
          trigger: { type: "schedule" },
        },
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.ingestionSchedules).toEqual({ daily_sync: "" });
  });

  it("returns empty object when no ingestion is defined", () => {
    const tpl = makeTemplate({ variables: {} });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.ingestionSchedules).toEqual({});
  });

  // --- Web auth ---

  it("detects OIDC web auth from template interfaces", () => {
    const tpl = makeTemplate({
      variables: {},
      interfaces: {
        auth: { web: { type: "oidc" } },
        adapters: ["web"],
      },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.webAuthEnabled).toBe(true);
  });

  it("returns false for web auth when not OIDC", () => {
    const tpl = makeTemplate({
      variables: {},
      interfaces: { adapters: ["web"] },
    });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.webAuthEnabled).toBe(false);
  });

  it("returns false for web auth when no interfaces", () => {
    const tpl = makeTemplate({ variables: {}, interfaces: undefined });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.webAuthEnabled).toBe(false);
  });

  // --- Template with no variables ---

  it("returns empty variableValues when template has no variables", () => {
    const tpl = makeTemplate({ variables: undefined as unknown as DeploymentTemplate["variables"] });
    const result = computeFormDefaults(tpl, "my-agent");
    expect(result.variableValues).toEqual({});
    expect(result.adapterCredentials).toEqual({});
  });

  // --- Full realistic template ---

  it("handles a realistic template with mixed variable types", () => {
    const tpl = makeTemplate({
      variables: {
        OPENAI_API_KEY: { targets: ["agent"], secret: true, optional: false, description: "API key" },
        MODEL: { default: "gpt-4o", targets: ["agent"], description: "Model to use", "display-as": "select", options: ["gpt-4o", "gpt-4o-mini", "claude-3"] },
        VERBOSE: { default: "false", targets: ["agent"], datatype: "boolean" },
        SLACK_BOT_TOKEN: { targets: ["interface.slack"], secret: true },
        SLACK_APP_TOKEN: { targets: ["interface.slack"], secret: true },
        SLACK_CONFIG: {
          default: JSON.stringify({ actionable_reactions: ["ticket"] }),
          targets: ["interface.slack"],
          optional: true,
          datatype: "object",
          fields: slackConfigFields,
        },
      },
      ingestion: {
        nightly: {
          image: "sync:latest",
          trigger: { type: "schedule", schedule: "0 2 * * *" },
        },
      },
      interfaces: {
        auth: { web: { type: "oidc" } },
        adapters: ["web"],
      },
    });

    const result = computeFormDefaults(tpl, "code-reviewer");

    expect(result.deployName).toBe("Code Reviewer");
    expect(result.selectedAdapters).toEqual(["web"]);
    expect(result.variableValues).toEqual({
      OPENAI_API_KEY: "",
      MODEL: "gpt-4o",
      VERBOSE: "false",
    });
    expect(result.adapterCredentials).toEqual({
      "SLACK_CONFIG.actionable_reactions": "ticket",
    });
    expect(result.ingestionSchedules).toEqual({ nightly: "0 2 * * *" });
    expect(result.webAuthEnabled).toBe(true);
  });
});
