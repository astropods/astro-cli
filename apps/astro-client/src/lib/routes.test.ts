import { describe, expect, it } from "vitest";
import { knowledgeDetailPath } from "./routes";

describe("knowledgeDetailPath", () => {
  it("encodes the store name and account independently", () => {
    expect(knowledgeDetailPath("docs/2026?draft", "platform team")).toBe(
      "/knowledge/docs%2F2026%3Fdraft?account=platform%20team",
    );
  });
});
