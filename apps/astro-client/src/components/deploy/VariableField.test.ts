import { describe, it, expect } from "vitest";
import { humanizeKey } from "./VariableField";

describe("humanizeKey", () => {
  it("capitalizes API correctly", () => {
    expect(humanizeKey("SLACK_API_KEY")).toBe("Slack API Key");
    expect(humanizeKey("API_KEY")).toBe("API Key");
    expect(humanizeKey("OPENAI_API_KEY")).toBe("OpenAI API Key");
  });

  it("capitalizes ID correctly", () => {
    expect(humanizeKey("WORKSPACE_ID")).toBe("Workspace ID");
    expect(humanizeKey("ORG_ID")).toBe("Org ID");
  });

  it("capitalizes IDs correctly", () => {
    expect(humanizeKey("WORKSPACE_IDS")).toBe("Workspace IDs");
    expect(humanizeKey("ORG_IDS")).toBe("Org IDs");
  });

  it("capitalizes URL correctly", () => {
    expect(humanizeKey("WEBHOOK_URL")).toBe("Webhook URL");
  });

  it("capitalizes OAuth correctly", () => {
    expect(humanizeKey("OAUTH_TOKEN")).toBe("OAuth Token");
  });

  it("capitalizes OpenAI correctly", () => {
    expect(humanizeKey("OPENAI_API_KEY")).toBe("OpenAI API Key");
  });

  it("capitalizes AI correctly", () => {
    expect(humanizeKey("AI_MODEL")).toBe("AI Model");
  });

  it("capitalizes LLM correctly", () => {
    expect(humanizeKey("LLM_ENDPOINT")).toBe("LLM Endpoint");
  });

  it("capitalizes DB correctly", () => {
    expect(humanizeKey("DB_CONNECTION_STRING")).toBe("DB Connection String");
  });

  it("capitalizes SDK correctly", () => {
    expect(humanizeKey("SDK_VERSION")).toBe("SDK Version");
  });

  it("capitalizes JWT correctly", () => {
    expect(humanizeKey("JWT_SECRET")).toBe("JWT Secret");
  });

  it("capitalizes GitHub correctly", () => {
    expect(humanizeKey("GITHUB_TOKEN")).toBe("GitHub Token");
  });

  it("handles plain keys without acronyms", () => {
    expect(humanizeKey("BOT_TOKEN")).toBe("Bot Token");
    expect(humanizeKey("MAX_TOKENS")).toBe("Max Tokens");
    expect(humanizeKey("WEB_REQUIRE_AUTH")).toBe("Web Require Auth");
  });
});
