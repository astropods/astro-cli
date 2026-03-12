import { describe, it, expect } from "vitest";
import { extractInitialValues } from "./extractInitialValues";
import type { DeploymentTemplate } from "@/lib/api";
import { mockTemplate } from "@/test/msw/handlers";

function makeTemplate(overrides: Partial<DeploymentTemplate> = {}): DeploymentTemplate {
  return { ...mockTemplate, ...overrides };
}

describe("extractInitialValues", () => {
  it("extracts deploy name from display_name", () => {
    const tpl = makeTemplate({ target: { ...mockTemplate.target, display_name: "My Bot" } });
    const result = extractInitialValues(tpl, "acme");
    expect(result.deployName).toBe("My Bot");
    expect(result.targetAccount).toBe("acme");
  });

  it("returns empty deployName when display_name is empty", () => {
    const tpl = makeTemplate({ target: { ...mockTemplate.target, display_name: "" } });
    const result = extractInitialValues(tpl, "acme");
    expect(result.deployName).toBe("");
  });

  it("returns empty deployName when display_name is undefined", () => {
    const tpl = makeTemplate({ target: { ...mockTemplate.target, display_name: undefined as unknown as string } });
    const result = extractInitialValues(tpl, "acme");
    expect(result.deployName).toBe("");
  });

  it("routes regular variables to variableValues", () => {
    const tpl = makeTemplate({
      variables: {
        OPENAI_API_KEY: { value: "sk-123", default: "", targets: ["agent"], secret: true, optional: false, description: "key" },
      },
    });
    const result = extractInitialValues(tpl, "acme");
    expect(result.variableValues).toEqual({ OPENAI_API_KEY: "sk-123" });
    expect(result.adapterCredentials).toEqual({});
  });

  it("routes adapter credentials (by key) to adapterCredentials", () => {
    const tpl = makeTemplate({
      variables: {
        SLACK_BOT_TOKEN: { value: "xoxb-test", default: "", targets: ["agent"], secret: true, optional: false, description: "slack" },
      },
    });
    const result = extractInitialValues(tpl, "acme");
    expect(result.adapterCredentials).toEqual({ SLACK_BOT_TOKEN: "xoxb-test" });
    expect(result.variableValues).toEqual({});
  });

  it("routes interface-targeted variables to adapterCredentials", () => {
    const tpl = makeTemplate({
      variables: {
        CUSTOM_TOKEN: { value: "tok", default: "", targets: ["interface.slack"], secret: true, optional: false, description: "custom" },
      },
    });
    const result = extractInitialValues(tpl, "acme");
    expect(result.adapterCredentials).toEqual({ CUSTOM_TOKEN: "tok" });
    expect(result.variableValues).toEqual({});
  });

  it("routes variable with undefined targets to variableValues", () => {
    const tpl = makeTemplate({
      variables: {
        PLAIN_VAR: { value: "val", default: "", targets: undefined as unknown as string[], secret: false, optional: false, description: "no targets" },
      },
    });
    const result = extractInitialValues(tpl, "acme");
    expect(result.variableValues).toEqual({ PLAIN_VAR: "val" });
    expect(result.adapterCredentials).toEqual({});
  });

  it("falls back to default when value is missing", () => {
    const tpl = makeTemplate({
      variables: {
        MY_VAR: { value: undefined as unknown as string, default: "fallback", targets: ["agent"], secret: false, optional: false, description: "v" },
      },
    });
    const result = extractInitialValues(tpl, "acme");
    expect(result.variableValues).toEqual({ MY_VAR: "fallback" });
  });

  it("falls back to empty string when both value and default are missing", () => {
    const tpl = makeTemplate({
      variables: {
        MY_VAR: { value: undefined as unknown as string, default: undefined as unknown as string, targets: ["agent"], secret: false, optional: false, description: "v" },
      },
    });
    const result = extractInitialValues(tpl, "acme");
    expect(result.variableValues).toEqual({ MY_VAR: "" });
  });

  it("extracts selectedAdapters from template interfaces", () => {
    const tpl = makeTemplate({ interfaces: { adapters: ["web", "slack"] } });
    const result = extractInitialValues(tpl, "acme");
    expect(result.selectedAdapters).toEqual(["web", "slack"]);
  });

  it("defaults selectedAdapters to [web] when interfaces missing", () => {
    const tpl = makeTemplate({ interfaces: undefined });
    const result = extractInitialValues(tpl, "acme");
    expect(result.selectedAdapters).toEqual(["web"]);
  });

  it("handles template with no variables", () => {
    const tpl = makeTemplate({ variables: undefined as unknown as DeploymentTemplate["variables"] });
    const result = extractInitialValues(tpl, "acme");
    expect(result.variableValues).toEqual({});
    expect(result.adapterCredentials).toEqual({});
  });

  it("splits mixed variables correctly", () => {
    const tpl = makeTemplate({
      variables: {
        OPENAI_API_KEY: { value: "sk-123", default: "", targets: ["agent"], secret: true, optional: false, description: "key" },
        SLACK_BOT_TOKEN: { value: "xoxb-abc", default: "", targets: ["agent"], secret: true, optional: false, description: "slack" },
        SENTRY_DSN: { value: "https://sentry.io/1", default: "", targets: ["agent"], secret: false, optional: true, description: "sentry" },
      },
      interfaces: { adapters: ["web", "slack"] },
    });
    const result = extractInitialValues(tpl, "team");
    expect(result.variableValues).toEqual({ OPENAI_API_KEY: "sk-123", SENTRY_DSN: "https://sentry.io/1" });
    expect(result.adapterCredentials).toEqual({ SLACK_BOT_TOKEN: "xoxb-abc" });
    expect(result.selectedAdapters).toEqual(["web", "slack"]);
    expect(result.targetAccount).toBe("team");
  });
});
