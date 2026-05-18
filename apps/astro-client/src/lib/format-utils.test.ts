import { describe, it, expect } from "vitest";
import { formatCost, formatCompact, formatLatency, formatDateShort } from "./format-utils";
import { buildPeriodParams } from "@/components/activity/ranges";

describe("formatCost", () => {
  it("formats millions", () => {
    expect(formatCost(2_500_000)).toBe("$2.50M");
  });

  it("formats thousands", () => {
    expect(formatCost(1_500)).toBe("$1.5K");
  });

  it("formats sub-cent values with 4 decimal places", () => {
    expect(formatCost(0.001)).toBe("$0.0010");
  });

  it("formats normal values with 2 decimal places", () => {
    expect(formatCost(3.5)).toBe("$3.50");
    expect(formatCost(0)).toBe("$0.00");
  });
});

describe("formatCompact", () => {
  it("formats millions", () => {
    expect(formatCompact(1_200_000)).toBe("1.2M");
  });

  it("formats thousands", () => {
    expect(formatCompact(5_000)).toBe("5.0K");
  });

  it("formats small numbers as-is", () => {
    expect(formatCompact(42)).toBe("42");
    expect(formatCompact(0)).toBe("0");
  });
});

describe("formatLatency", () => {
  it("formats seconds for values >= 1000ms", () => {
    expect(formatLatency(1_500)).toBe("1.5s");
    expect(formatLatency(1_000)).toBe("1.0s");
  });

  it("formats milliseconds for values < 1000ms", () => {
    expect(formatLatency(250)).toBe("250ms");
    expect(formatLatency(0)).toBe("0ms");
  });
});

describe("formatDateShort", () => {
  it("formats a date string without time", () => {
    expect(formatDateShort("2024-01-15")).toBe("Jan 15");
  });

  it("formats an ISO timestamp", () => {
    expect(formatDateShort("2024-06-01T12:00:00Z")).toBe("Jun 1");
  });
});

describe("buildPeriodParams", () => {
  it("returns empty object for 'all'", () => {
    expect(buildPeriodParams("all")).toEqual({});
  });

  it("returns from/to for '7d'", () => {
    const { from, to } = buildPeriodParams("7d");
    expect(from).toBeDefined();
    expect(to).toBeDefined();
    const diff = new Date(to!).getTime() - new Date(from!).getTime();
    const days = diff / (1000 * 60 * 60 * 24);
    expect(days).toBeCloseTo(7, 0);
  });

  it("returns from/to for '30d'", () => {
    const { from, to } = buildPeriodParams("30d");
    const diff = new Date(to!).getTime() - new Date(from!).getTime();
    const days = diff / (1000 * 60 * 60 * 24);
    expect(days).toBeCloseTo(30, 0);
  });

  it("from is UTC midnight", () => {
    const { from } = buildPeriodParams("7d");
    const d = new Date(from!);
    expect(d.getUTCHours()).toBe(0);
    expect(d.getUTCMinutes()).toBe(0);
    expect(d.getUTCSeconds()).toBe(0);
  });
});
