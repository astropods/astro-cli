import { describe, it, expect } from "vitest";
import { comparePersonalFirst } from "./account-order";

const name = (a: { name: string }) => a.name;

describe("comparePersonalFirst", () => {
  it("puts the personal account first, then the rest alphabetically by name", () => {
    const accounts = [
      { type: "organization", name: "beta" },
      { type: "organization", name: "acme" },
      { type: "personal", name: "zach" },
    ];
    expect([...accounts].sort(comparePersonalFirst).map(name)).toEqual(["zach", "acme", "beta"]);
  });

  it("orders organizations alphabetically when there is no personal account", () => {
    const accounts = [
      { type: "organization", name: "gamma" },
      { type: "organization", name: "alpha" },
    ];
    expect([...accounts].sort(comparePersonalFirst).map(name)).toEqual(["alpha", "gamma"]);
  });
});
