import { describe, it, expect } from "vitest";
import { isValidCron, isValidCronField } from "./cron-validation";

describe("isValidCronField", () => {
  it("accepts wildcard", () => {
    expect(isValidCronField("*", 0, 59)).toBe(true);
  });

  it("accepts a number within range", () => {
    expect(isValidCronField("0", 0, 59)).toBe(true);
    expect(isValidCronField("59", 0, 59)).toBe(true);
    expect(isValidCronField("23", 0, 23)).toBe(true);
  });

  it("rejects a number outside range", () => {
    expect(isValidCronField("60", 0, 59)).toBe(false);
    expect(isValidCronField("-1", 0, 59)).toBe(false);
    expect(isValidCronField("24", 0, 23)).toBe(false);
    expect(isValidCronField("0", 1, 31)).toBe(false);
  });

  it("accepts valid step values", () => {
    expect(isValidCronField("*/5", 0, 59)).toBe(true);
    expect(isValidCronField("*/15", 0, 59)).toBe(true);
    expect(isValidCronField("*/6", 0, 23)).toBe(true);
  });

  it("rejects step of zero", () => {
    expect(isValidCronField("*/0", 0, 59)).toBe(false);
  });

  it("rejects step exceeding max", () => {
    expect(isValidCronField("*/60", 0, 59)).toBe(false);
    expect(isValidCronField("*/24", 0, 23)).toBe(false);
  });

  it("rejects non-numeric values", () => {
    expect(isValidCronField("abc", 0, 59)).toBe(false);
    expect(isValidCronField("*/abc", 0, 59)).toBe(false);
  });
});

describe("isValidCron", () => {
  it("accepts a basic 5-field expression", () => {
    expect(isValidCron("* * * * *")).toBe(true);
    expect(isValidCron("0 0 * * *")).toBe(true);
    expect(isValidCron("30 14 1 6 3")).toBe(true);
  });

  it("accepts step expressions", () => {
    expect(isValidCron("*/15 * * * *")).toBe(true);
    expect(isValidCron("0 */6 * * *")).toBe(true);
    expect(isValidCron("*/5 */2 * * *")).toBe(true);
  });

  it("rejects too few fields", () => {
    expect(isValidCron("* * *")).toBe(false);
    expect(isValidCron("0 0 * *")).toBe(false);
  });

  it("rejects too many fields", () => {
    expect(isValidCron("* * * * * *")).toBe(false);
  });

  it("rejects empty string", () => {
    expect(isValidCron("")).toBe(false);
  });

  it("rejects out-of-range values per field", () => {
    expect(isValidCron("60 * * * *")).toBe(false);
    expect(isValidCron("* 24 * * *")).toBe(false);
    expect(isValidCron("* * 0 * *")).toBe(false);
    expect(isValidCron("* * * 13 *")).toBe(false);
    expect(isValidCron("* * * * 7")).toBe(false);
  });

  it("handles extra whitespace gracefully", () => {
    expect(isValidCron("  0   0   *   *   * ")).toBe(true);
  });
});
