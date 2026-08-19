import { describe, expect, it } from "vitest";
import { topSegmentKey } from "./stacked-bars";

describe("topSegmentKey", () => {
  const order = ["work", "personal", "ambiguous"];

  it("picks the last series in stack order that has a value", () => {
    expect(topSegmentKey({ work: 3, personal: 1, ambiguous: 2 }, order)).toBe("ambiguous");
  });

  // Rounding the last declared series instead would put the corner on a
  // zero-height rect and leave the visible top square.
  it("skips series that are zero on this row", () => {
    expect(topSegmentKey({ work: 3, personal: 1, ambiguous: 0 }, order)).toBe("personal");
    expect(topSegmentKey({ work: 3, personal: 0, ambiguous: 0 }, order)).toBe("work");
  });

  // Hiding a series removes it from stackOrder, so the cap moves down.
  it("follows the visible set when the legend filters series out", () => {
    expect(topSegmentKey({ work: 3, personal: 1, ambiguous: 2 }, ["work", "personal"])).toBe("personal");
    expect(topSegmentKey({ work: 3, personal: 1, ambiguous: 2 }, ["work"])).toBe("work");
  });

  // A row built for a different range carries keys this one does not.
  it("ignores keys absent from the row", () => {
    expect(topSegmentKey({ work: 2 }, order)).toBe("work");
  });

  it("returns empty when nothing is drawn", () => {
    expect(topSegmentKey({ work: 0, personal: 0, ambiguous: 0 }, order)).toBe("");
    expect(topSegmentKey(undefined, order)).toBe("");
    expect(topSegmentKey({ work: 1 }, [])).toBe("");
  });
});
