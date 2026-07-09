import { describe, it, expect } from "vitest";
import { classify, roleRank, brandIconId, type Role } from "./classify";

describe("classify", () => {
  it("maps the literal components", () => {
    expect(classify("agent")).toBe("agent");
    expect(classify("collector")).toBe("collector");
  });

  it("maps prefixed components by category", () => {
    expect(classify("knowledge-postgres")).toBe("knowledge");
    expect(classify("model-ollama")).toBe("model");
    expect(classify("tool-github")).toBe("integration");
    expect(classify("ingestion-acme")).toBe("ingestion");
  });

  it("maps bare provider names (legacy payloads)", () => {
    expect(classify("postgres")).toBe("knowledge");
    expect(classify("redis")).toBe("knowledge");
    expect(classify("ollama")).toBe("model");
  });

  it("is case-insensitive", () => {
    expect(classify("Knowledge-Postgres")).toBe("knowledge");
    expect(classify("AGENT")).toBe("agent");
  });

  it("falls back to ingestion for bare Jobs/CronJobs", () => {
    expect(classify("", "Job")).toBe("ingestion");
    expect(classify(undefined, "CronJob")).toBe("ingestion");
  });

  it("returns other for unknown components", () => {
    expect(classify("")).toBe("other");
    expect(classify("mystery", "Deployment")).toBe("other");
  });
});

describe("roleRank", () => {
  it("orders roles by the left-to-right flow of the layout", () => {
    const roles: Role[] = ["other", "collector", "model", "agent", "knowledge", "ingestion"];
    const sorted = [...roles].sort((a, b) => roleRank(a) - roleRank(b));
    expect(sorted).toEqual(["ingestion", "knowledge", "agent", "model", "collector", "other"]);
  });
});

describe("brandIconId", () => {
  it("prefers the declared provider over the component name", () => {
    // Component keyed by an arbitrary name, but the provider is authoritative.
    expect(brandIconId("knowledge", "postgres", "knowledge-mydb")).toBe("postgres");
    expect(brandIconId("model", "ollama", "model-primary")).toBe("ollama");
  });

  it("falls back to the component suffix when no provider is given", () => {
    expect(brandIconId("knowledge", undefined, "knowledge-qdrant")).toBe("qdrant");
    expect(brandIconId("integration", "", "tool-github")).toBe("github");
    expect(brandIconId("knowledge", undefined, "postgres")).toBe("postgres");
  });

  it("returns null when neither provider nor name maps to a shipped icon", () => {
    expect(brandIconId("knowledge", "", "knowledge-mystore")).toBeNull();
    expect(brandIconId("model", "vllm", "model-vllm")).toBeNull();
  });

  it("returns null for roles without brand icons", () => {
    expect(brandIconId("agent", "", "agent")).toBeNull();
    expect(brandIconId("collector", "", "collector")).toBeNull();
    expect(brandIconId("ingestion", undefined, "ingestion-crawler")).toBeNull();
  });
});
