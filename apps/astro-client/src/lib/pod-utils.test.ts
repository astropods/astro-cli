import { describe, it, expect } from "vitest";
import { getPodStableName, getPodDisplayName } from "./pod-utils";

describe("getPodStableName", () => {
  it("strips replicaset hash and pod hash from a standard pod name", () => {
    expect(getPodStableName("myagent-agent-7f8b9c4d5-x2k9p")).toBe("myagent-agent");
  });

  it("handles multi-hyphen base names", () => {
    expect(getPodStableName("clawbot-ai-agent-abc12-z9y8x")).toBe("clawbot-ai-agent");
  });

  it("returns the original name if it has two or fewer segments", () => {
    expect(getPodStableName("myagent")).toBe("myagent");
    expect(getPodStableName("my-agent")).toBe("my-agent");
  });
});

describe("getPodDisplayName", () => {
  it("preserves template name dashes and spaces the component", () => {
    expect(getPodDisplayName("clawbot-ai-agent", "clawbot-ai")).toBe("clawbot-ai agent");
  });

  it("spaces multi-word components after the template name", () => {
    expect(getPodDisplayName("clawbot-ai-otel-collector", "clawbot-ai")).toBe("clawbot-ai otel collector");
  });

  it("handles simple template names", () => {
    expect(getPodDisplayName("myagent-messaging", "myagent")).toBe("myagent messaging");
  });

  it("returns the stable name as-is without blueprintName", () => {
    expect(getPodDisplayName("clawbot-ai-agent")).toBe("clawbot-ai-agent");
  });

  it("returns the stable name as-is when it doesn't match blueprintName", () => {
    expect(getPodDisplayName("something-else", "clawbot-ai")).toBe("something-else");
  });
});
