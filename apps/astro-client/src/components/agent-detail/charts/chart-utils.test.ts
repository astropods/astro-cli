import { afterEach, describe, expect, it, vi } from "vitest";
import { buildTimeParams, getEvenlySpacedDateTicks } from "./chart-utils";

afterEach(() => {
  vi.useRealTimers();
});

describe("buildTimeParams", () => {
  it("uses one stable window throughout a minute", () => {
    vi.useFakeTimers();
    vi.setSystemTime("2026-07-15T12:34:01.125Z");
    const first = buildTimeParams(7, { granularity: "hour" });

    vi.setSystemTime("2026-07-15T12:34:59.999Z");
    const second = buildTimeParams(7, { granularity: "hour" });

    expect(first).toEqual(second);
    expect(first).toEqual({
      start_time: "2026-07-08T12:34:00.000Z",
      end_time: "2026-07-15T12:34:00.000Z",
      granularity: "hour",
    });
  });
});

describe("getEvenlySpacedDateTicks", () => {
  const points = (count: number) =>
    Array.from({ length: count }, (_, index) => ({
      label: `Day ${index + 1}`,
    }));

  it.each([
    [7, [0, 1, 2, 3, 4, 5, 6]],
    [14, [0, 2, 4, 7, 9, 11, 13]],
    [30, [0, 5, 10, 15, 19, 24, 29]],
  ])(
    "snaps the seven ticks and labels to real day indexes across a %d-day range",
    (days, positions) => {
      const ticks = getEvenlySpacedDateTicks(points(days));

      expect(ticks.map((tick) => tick.position)).toEqual(positions);
      expect(ticks.map((tick) => tick.label)).toEqual(
        positions.map((position) => `Day ${position + 1}`),
      );
    },
  );

  it("handles an empty range", () => {
    expect(getEvenlySpacedDateTicks([])).toEqual([]);
  });
});
