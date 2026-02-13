import { describe, test, expect } from "bun:test";
import { generateIdentity } from "./generate";

describe("generateIdentity", () => {
  test("same seed produces identical output", () => {
    const a = generateIdentity({ seed: "test-seed" });
    const b = generateIdentity({ seed: "test-seed" });
    expect(a).toBe(b);
  });

  test("deterministic across multiple calls", () => {
    const results = Array.from({ length: 100 }, () =>
      generateIdentity({ seed: "stable" }),
    );
    const unique = new Set(results);
    expect(unique.size).toBe(1);
  });

  test("different seeds produce different output", () => {
    const a = generateIdentity({ seed: "alice" });
    const b = generateIdentity({ seed: "bob" });
    expect(a).not.toBe(b);
  });

  test("size parameter is respected", () => {
    const small = generateIdentity({ seed: "test", size: 64 });
    const large = generateIdentity({ seed: "test", size: 256 });
    expect(small).toContain('width="64"');
    expect(small).toContain('height="64"');
    expect(large).toContain('width="256"');
    expect(large).toContain('height="256"');
  });

  test("output is valid SVG", () => {
    const svg = generateIdentity({ seed: "svg-check" });
    expect(svg).toStartWith("<svg");
    expect(svg).toEndWith("</svg>");
    expect(svg).toContain('xmlns="http://www.w3.org/2000/svg"');
  });

  test("default size is 128", () => {
    const svg = generateIdentity({ seed: "default-size" });
    expect(svg).toContain('width="128"');
    expect(svg).toContain('height="128"');
  });
});
