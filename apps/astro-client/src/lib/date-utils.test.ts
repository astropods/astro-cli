import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { dayKeyFromISO, dayKeysForRange, formatDayKey, formatShortDate, formatShortDateLocal, utcDayKey } from "./date-utils";

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

describe("formatting billing dates west of Greenwich", () => {
  const tz = process.env.TZ;
  beforeAll(() => {
    process.env.TZ = "America/Los_Angeles";
  });
  afterAll(() => {
    process.env.TZ = tz;
  });

  // The provider sends period boundaries as UTC midnight. Rendering those in
  // local time moved every one of them to the previous day for the Americas,
  // so an invoice for the 11th read as the 10th and every chart bar was
  // labelled a day early.
  it("names the UTC day for a period boundary", () => {
    expect(formatShortDate("2026-09-11T00:00:00Z")).toBe("Sep 11, 2026");
  });

  it("names the UTC day for a chart bucket key", () => {
    expect(formatDayKey("2026-08-11")).toBe("Aug 11");
  });

  // formatShortDateLocal is the opposite call for the opposite kind of
  // timestamp: a real instant a user experienced in their own evening, not a
  // provider-reported UTC-midnight boundary. It should stay on the viewer's
  // own day instead of jumping to the next UTC day like formatShortDate would.
  it("names the viewer's own day for a late-evening instant, not the UTC day", () => {
    expect(formatShortDateLocal("2026-08-24T03:00:00Z")).toBe("Aug 23, 2026");
  });
});
