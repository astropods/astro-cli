import { describe, expect, it } from "vitest";
import { dayKeyFromISO, dayKeysForRange, utcDayKey } from "./date-utils";

describe("dayKeysForRange", () => {
  // The axis has to end where the data ends. The rollup-backed path reports
  // windows ending at the last complete day, so an axis anchored on today drew
  // a trailing bucket that could never be filled.
  it("ends on the given day rather than on today", () => {
    expect(dayKeysForRange(3, "2026-06-08")).toEqual([
      "2026-06-06",
      "2026-06-07",
      "2026-06-08",
    ]);
  });

  it("walks UTC across a month boundary", () => {
    expect(dayKeysForRange(3, "2026-07-01")).toEqual([
      "2026-06-29",
      "2026-06-30",
      "2026-07-01",
    ]);
  });

  it("falls back to a window ending today when no end day is given", () => {
    const keys = dayKeysForRange(7);
    expect(keys).toHaveLength(7);
    expect(keys[keys.length - 1]).toBe(utcDayKey(new Date()));
  });

  // A malformed end day must not produce an axis of "NaN-NaN-NaN" keys, which
  // would silently match no data and render an empty chart.
  it("falls back to today when the end day is unparseable", () => {
    const keys = dayKeysForRange(2, "not-a-date");
    expect(keys).toHaveLength(2);
    expect(keys[keys.length - 1]).toBe(utcDayKey(new Date()));
  });
});

describe("dayKeyFromISO", () => {
  it("takes the UTC day from an RFC3339 timestamp", () => {
    expect(dayKeyFromISO("2026-06-08T23:59:59.999Z")).toBe("2026-06-08");
  });

  it("returns undefined for missing or unparseable input", () => {
    expect(dayKeyFromISO(undefined)).toBeUndefined();
    expect(dayKeyFromISO("")).toBeUndefined();
    expect(dayKeyFromISO("nonsense")).toBeUndefined();
  });
});
