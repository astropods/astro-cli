import { describe, it, expect } from "vitest";
import { normalizeLevel, levelColorClass, formatLogTimestamp } from "./log-utils";

describe("normalizeLevel", () => {
  it("maps lowercase level strings", () => {
    expect(normalizeLevel("info")).toBe("INFO");
    expect(normalizeLevel("debug")).toBe("DEBUG");
    expect(normalizeLevel("warn")).toBe("WARN");
    expect(normalizeLevel("error")).toBe("ERROR");
    expect(normalizeLevel("fatal")).toBe("FATAL");
    expect(normalizeLevel("trace")).toBe("TRACE");
  });

  it("maps uppercase level strings", () => {
    expect(normalizeLevel("INFO")).toBe("INFO");
    expect(normalizeLevel("WARN")).toBe("WARN");
    expect(normalizeLevel("ERROR")).toBe("ERROR");
  });

  it("maps aliases", () => {
    expect(normalizeLevel("warning")).toBe("WARN");
    expect(normalizeLevel("WARNING")).toBe("WARN");
    expect(normalizeLevel("err")).toBe("ERROR");
    expect(normalizeLevel("crit")).toBe("FATAL");
    expect(normalizeLevel("critical")).toBe("FATAL");
  });

  it("defaults to INFO for null", () => {
    expect(normalizeLevel(null)).toBe("INFO");
  });

  it("defaults to INFO for empty string", () => {
    expect(normalizeLevel("")).toBe("INFO");
  });

  it("defaults to INFO for unknown values", () => {
    expect(normalizeLevel("verbose")).toBe("INFO");
    expect(normalizeLevel("notice")).toBe("INFO");
  });
});

describe("levelColorClass", () => {
  it("returns red for ERROR and FATAL", () => {
    expect(levelColorClass("error")).toBe("text-coral-600");
    expect(levelColorClass("ERROR")).toBe("text-coral-600");
    expect(levelColorClass("fatal")).toBe("text-coral-600");
  });

  it("returns yellow for WARN", () => {
    expect(levelColorClass("warn")).toBe("text-yellow-600");
    expect(levelColorClass("warning")).toBe("text-yellow-600");
  });

  it("returns blue for INFO", () => {
    expect(levelColorClass("info")).toBe("text-blue-500");
    expect(levelColorClass(null)).toBe("text-blue-500");
    expect(levelColorClass("")).toBe("text-blue-500");
  });

  it("returns faint for DEBUG and TRACE", () => {
    expect(levelColorClass("debug")).toBe("text-faint-foreground");
    expect(levelColorClass("trace")).toBe("text-faint-foreground");
  });
});

describe("formatLogTimestamp", () => {
  it("formats ISO timestamp to date time.millis", () => {
    expect(formatLogTimestamp("2026-04-13T21:48:08.470Z")).toBe("2026-04-13 21:48:08.470");
  });

  it("pads missing milliseconds with zeros", () => {
    expect(formatLogTimestamp("2026-04-13T21:48:08Z")).toBe("2026-04-13 21:48:08.000");
  });

  it("truncates sub-millisecond precision", () => {
    expect(formatLogTimestamp("2026-04-13T21:48:08.1234567Z")).toBe("2026-04-13 21:48:08.123");
  });

  it("returns em-dash for null", () => {
    expect(formatLogTimestamp(null)).toBe("—");
  });

  it("returns the original string if it cannot be parsed", () => {
    expect(formatLogTimestamp("not-a-timestamp")).toBe("not-a-timestamp");
  });

  it("strips negative UTC offset and preserves local time", () => {
    expect(formatLogTimestamp("2026-04-13T17:48:08.470-04:00")).toBe("2026-04-13 17:48:08.470");
  });

  it("strips positive UTC offset and preserves local time", () => {
    expect(formatLogTimestamp("2026-04-14T03:48:08.470+05:30")).toBe("2026-04-14 03:48:08.470");
  });
});
