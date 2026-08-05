import { afterEach, describe, expect, it, vi } from "vitest";
import { formatLongTimeAgo } from "./time-format";

afterEach(() => vi.useRealTimers());

describe("formatLongTimeAgo", () => {
  it("uses full relative units from minutes through years", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-05T12:00:00Z"));

    expect(formatLongTimeAgo("2026-08-05T11:48:00Z")).toBe("12 minutes ago");
    expect(formatLongTimeAgo("2026-08-05T09:00:00Z")).toBe("3 hours ago");
    expect(formatLongTimeAgo("2026-07-31T12:00:00Z")).toBe("5 days ago");
    expect(formatLongTimeAgo("2026-05-05T12:00:00Z")).toBe("3 months ago");
    expect(formatLongTimeAgo("2024-08-05T12:00:00Z")).toBe("2 years ago");
  });

  it("switches from months to years at twelve display months", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-05T12:00:00Z"));

    expect(formatLongTimeAgo("2025-08-11T12:00:00Z")).toBe("11 months ago");
    expect(formatLongTimeAgo("2025-08-10T12:00:00Z")).toBe("1 year ago");
    expect(formatLongTimeAgo("2025-08-06T12:00:00Z")).toBe("1 year ago");
  });

  it("handles immediate, future, and invalid timestamps", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-05T12:00:00Z"));

    expect(formatLongTimeAgo("2026-08-05T11:59:45Z")).toBe("Just now");
    expect(formatLongTimeAgo("2026-08-05T12:00:15Z")).toBe("Just now");
    expect(formatLongTimeAgo("2026-08-05T12:01:00Z")).toBe("—");
    expect(formatLongTimeAgo("not-a-date")).toBe("—");
  });
});
