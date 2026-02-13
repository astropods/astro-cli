import { describe, test, expect } from "bun:test";
import { hash, createRng } from "./rng";

describe("hash", () => {
  test("same string produces same hash", () => {
    expect(hash("hello")).toBe(hash("hello"));
  });

  test("different strings produce different hashes", () => {
    expect(hash("hello")).not.toBe(hash("world"));
  });
});

describe("createRng", () => {
  test("same seed produces same sequence", () => {
    const a = createRng(42);
    const b = createRng(42);
    for (let i = 0; i < 100; i++) {
      expect(a()).toBe(b());
    }
  });

  test("different seeds produce different sequences", () => {
    const a = createRng(1);
    const b = createRng(2);
    const match = Array.from({ length: 20 }, () => a() === b());
    expect(match.every(Boolean)).toBe(false);
  });

  test("output is in [0, 1)", () => {
    const rng = createRng(123);
    for (let i = 0; i < 1000; i++) {
      const v = rng();
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThan(1);
    }
  });
});
