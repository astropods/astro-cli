import { describe, expect, it } from "bun:test";
import { generateCard, getCardDimensions } from "./index";
import type { CardData } from "./types";

const sampleData: CardData = {
  name: "my-agent",
  account: "acme",
  avatar: { svg: `<rect width="128" height="128" fill="red"/>` },
  description: "A helpful agent that does things and helps people with stuff.",
  tags: ["assistant", "productivity"],
  heartCount: 42,
  colors: { background: "#1a1a2e", foreground: "#e8e8ec", accent: "#6366f1", accentLight: "#a5b4fc" },
};

describe("generateCard", () => {
  it("returns a valid SVG string", () => {
    const svg = generateCard(sampleData);
    expect(svg).toStartWith("<svg");
    expect(svg).toEndWith("</svg>");
  });

  it("uses provided background color", () => {
    const svg = generateCard(sampleData);
    expect(svg).toContain("#1a1a2e");
  });

  it("uses default colors when none provided", () => {
    const data: CardData = { name: "bare", account: "test" };
    const svg = generateCard(data);
    expect(svg).toContain("#1a1a2e");
  });

  it("is deterministic for the same input", () => {
    const a = generateCard(sampleData);
    const b = generateCard(sampleData);
    expect(a).toBe(b);
  });
});

describe("getCardDimensions", () => {
  it("returns correct dimensions", () => {
    expect(getCardDimensions()).toEqual({ width: 350, height: 560 });
  });
});
