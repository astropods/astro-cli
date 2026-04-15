import { describe, it, expect } from "vitest";
import { labelFromKey } from "./VariableField";

describe("labelFromKey", () => {
  it("capitalizes API correctly", () => {
    expect(labelFromKey("SLACK_API_KEY")).toBe("Slack API Key");
    expect(labelFromKey("API_KEY")).toBe("API Key");
    expect(labelFromKey("OPENAI_API_KEY")).toBe("OpenAI API Key");
  });

  it("capitalizes ID correctly", () => {
    expect(labelFromKey("WORKSPACE_ID")).toBe("Workspace ID");
    expect(labelFromKey("ORG_ID")).toBe("Org ID");
  });

  it("capitalizes IDs correctly", () => {
    expect(labelFromKey("WORKSPACE_IDS")).toBe("Workspace IDs");
    expect(labelFromKey("ORG_IDS")).toBe("Org IDs");
  });

  it("capitalizes URL correctly", () => {
    expect(labelFromKey("WEBHOOK_URL")).toBe("Webhook URL");
  });

  it("capitalizes OAuth correctly", () => {
    expect(labelFromKey("OAUTH_TOKEN")).toBe("OAuth Token");
  });

  it("capitalizes OpenAI correctly", () => {
    expect(labelFromKey("OPENAI_API_KEY")).toBe("OpenAI API Key");
  });

  it("capitalizes AI correctly", () => {
    expect(labelFromKey("AI_MODEL")).toBe("AI Model");
  });

  it("capitalizes LLM correctly", () => {
    expect(labelFromKey("LLM_ENDPOINT")).toBe("LLM Endpoint");
  });

  it("capitalizes DB correctly", () => {
    expect(labelFromKey("DB_CONNECTION_STRING")).toBe("DB Connection String");
  });

  it("capitalizes SDK correctly", () => {
    expect(labelFromKey("SDK_VERSION")).toBe("SDK Version");
  });

  it("capitalizes JWT correctly", () => {
    expect(labelFromKey("JWT_SECRET")).toBe("JWT Secret");
  });

  it("capitalizes GitHub correctly", () => {
    expect(labelFromKey("GITHUB_TOKEN")).toBe("GitHub Token");
  });

  it("handles plain keys without acronyms", () => {
    expect(labelFromKey("BOT_TOKEN")).toBe("Bot Token");
    expect(labelFromKey("MAX_TOKENS")).toBe("Max Tokens");
    expect(labelFromKey("WEB_REQUIRE_AUTH")).toBe("Web Require Auth");
  });
});
