import { afterEach, describe, expect, it, vi } from "vitest";
import { buildTimeParams } from "./chart-utils";

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
