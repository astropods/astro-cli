import { describe, expect, it } from "bun:test";
import { generateCard, getCardDimensions, DEFAULT_COLORS } from "./index";
import type { CardData } from "./types";
import { NAME_MAX_CHARS } from "./variants/standard";

const sampleData: CardData = {
  name: "my-agent",
  account: "acme",
  avatar: { svg: `<rect width="128" height="128" fill="red"/>` },
  description: "A helpful agent that does things and helps people with stuff.",
  tags: ["assistant", "productivity"],
  heartCount: 42,
  colors: { background: "#1a1a2e", foreground: "#e8e8ec", accent: "#6366f1", accentLight: "#a5b4fc", glow: "#b4bfff" },
};

describe("generateCard", () => {
  it("uses a three-line badge name budget", () => {
    expect(NAME_MAX_CHARS).toBe(42);
  });

  it("returns a valid SVG string", () => {
    const svg = generateCard(sampleData);
    expect(svg).toStartWith("<svg");
    expect(svg).toEndWith("</svg>");
  });

  it("uses provided background color", () => {
    const svg = generateCard(sampleData);
    expect(svg).toContain("#1a1a2e");
  });

  it("uses default teal colors when none provided", () => {
    const data: CardData = { name: "bare", account: "test" };
    const svg = generateCard(data);
    expect(svg).toContain(DEFAULT_COLORS.background);
    expect(svg).toContain(DEFAULT_COLORS.accent);
  });

  it("is deterministic for the same input", () => {
    const a = generateCard(sampleData);
    const b = generateCard(sampleData);
    expect(a).toBe(b);
  });

  it("caps and wraps long agent names within the badge name block", () => {
    const name = "LongAgentNameLongAgentNameLongAgentNameLongAgentName";
    const svg = generateCard({ ...sampleData, name, displayName: undefined });
    const nameBlock = svg.split(`<line x1="0"`)[0];
    const lines = [...nameBlock.matchAll(/text-anchor="middle" letter-spacing="0"[^>]*>([^<]+)<\/text>/g)]
      .map((match) => match[1]);

    expect(lines.map((line) => line.replace(/-$/, "").length)).toEqual([14, 14, 14]);
    expect(lines.join("").replace(/-/g, "")).toBe(name.slice(0, 42));
    expect(lines.join("")).not.toContain("\u2026");
  });

  it("prefers natural title breaks when the capped name still fits", () => {
    const name = "ProjectAgent/SlackBotDeepDive";
    const svg = generateCard({ ...sampleData, name, displayName: undefined });
    const nameBlock = svg.split(`<line x1="0"`)[0];
    const lines = [...nameBlock.matchAll(/text-anchor="middle" letter-spacing="0"[^>]*>([^<]+)<\/text>/g)]
      .map((match) => match[1]);

    expect(lines).toEqual(["ProjectAgent/", "SlackBotDeep", "Dive"]);
  });

  it("hyphen-wraps long FROM values so the full origin stays on the badge", () => {
    const fromValue = "testaccountnamewithsomewidth/verylongagentnamethatshouldhyphenwrapcleanlyandremainvisible";
    const svg = generateCard({
      ...sampleData,
      stats: [
        {
          label: "From",
          value: fromValue,
          wrap: true,
        },
      ],
    });
    const valueLines = [...svg.matchAll(/class="stat-value-line"[^>]*>([^<]+)<\/tspan>/g)]
      .map((match) => match[1]);

    expect(valueLines.length).toBeGreaterThan(1);
    expect(valueLines.join("").replace(/-/g, "")).toBe(fromValue);
    expect(svg).not.toContain("\u2026");
    expect(svg).toContain("stat-value-clip-0");
  });

  it("only hyphen-wraps stat values that opt in", () => {
    const fromValue = "testaccountnamewithsomewidth/verylongagentnamethatshouldhyphenwrapcleanlyandremainvisible";
    const svg = generateCard({
      ...sampleData,
      stats: [
        {
          label: "From",
          value: fromValue,
        },
      ],
    });

    expect(svg).not.toContain("stat-value-line");
  });

  it("keeps worst-case metadata above the barcode", () => {
    const name = "LongAgentNameLongAgentNameLongAgentNameLongAgentName";
    const fromValue = "testaccountnamewithsomewidth/verylongagentnamethatshouldhyphenwrapcleanlyandremainvisible";
    const svg = generateCard({
      ...sampleData,
      name,
      displayName: undefined,
      barcodeId: "fl6-bid-dzk",
      qrUrl: "https://example.com/acme/my-agent",
      stats: [
        { label: "Deployed", value: "Jul 8, 2026" },
        { label: "From", value: fromValue, wrap: true },
      ],
      integrations: [
        { name: "SuperLongIntegrationOne" },
        { name: "SuperLongIntegrationTwo" },
        { name: "SuperLongIntegrationThree" },
      ],
    });
    const barcodeY = 484;
    const beforeBarcode = svg.split(`<rect x="20" y="${barcodeY}"`)[0];
    const yValues = [...beforeBarcode.matchAll(/\s(?:y|y1)="([0-9.]+)"/g)].map((match) => Number(match[1]));

    expect(Math.max(...yValues)).toBeLessThan(barcodeY);
  });
});

describe("getCardDimensions", () => {
  it("returns correct dimensions", () => {
    expect(getCardDimensions()).toEqual({ width: 350, height: 560 });
  });
});
