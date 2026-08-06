import { describe, it, expect } from "vitest";
import {
  buildInsightsQueryParams,
  insightsFreshnessNote,
  resolveInsightsDateLabel,
  shouldRevalidate,
} from "./Insights";

// Insights opts most search-param changes out of loader revalidation
// (range/agent toggles handle the new query key client-side). Only
// programmatic revalidations — the signal setActiveAccount sends — should
// re-run the loader.
describe("Insights shouldRevalidate", () => {
  const args = (currentUrl: string, nextUrl: string, defaultShouldRevalidate = false) => ({
    currentUrl: new URL(currentUrl),
    nextUrl: new URL(nextUrl),
    defaultShouldRevalidate,
  });

  it("revalidates on programmatic revalidation (currentUrl === nextUrl) — the org-switch signal", () => {
    expect(shouldRevalidate(args("http://x/insights", "http://x/insights"))).toBe(true);
    expect(shouldRevalidate(args("http://x/insights?range=7d", "http://x/insights?range=7d"))).toBe(true);
  });

  it("skips revalidation on any search-param change within the page — view, range, agents, users", () => {
    expect(shouldRevalidate(args("http://x/insights", "http://x/insights?range=7d"))).toBe(false);
    expect(shouldRevalidate(args("http://x/insights?range=7d", "http://x/insights?range=14d"))).toBe(false);
    expect(shouldRevalidate(args("http://x/insights?range=7d", "http://x/insights?range=7d&agents=a,b"))).toBe(false);
    // View toggle is a pure client-side flip — both views share chart data
    // and tables fetch separately, so the loader never needs to re-run on
    // `?view=` changes.
    expect(shouldRevalidate(args("http://x/insights", "http://x/insights?view=users"))).toBe(false);
    expect(shouldRevalidate(args("http://x/insights?view=users", "http://x/insights"))).toBe(false);
    expect(shouldRevalidate(args("http://x/insights?range=7d", "http://x/insights?range=14d&view=users"))).toBe(false);
  });

  it("defers to defaultShouldRevalidate when the pathname actually changed", () => {
    expect(shouldRevalidate(args("http://x/agents", "http://x/insights", true))).toBe(true);
    expect(shouldRevalidate(args("http://x/agents", "http://x/insights", false))).toBe(false);
  });
});

describe("buildInsightsQueryParams", () => {
  it("defaults both table limits to 25", () => {
    const params = buildInsightsQueryParams({});
    expect(params.agents_limit).toBe("25");
    expect(params.people_limit).toBe("25");
    expect(params.agents_offset).toBe("0");
    expect(params.people_offset).toBe("0");
  });

  it("forwards custom table limits and omits empty search", () => {
    const params = buildInsightsQueryParams({ agentsLimit: 35, peopleLimit: 40, query: "  " });
    expect(params.agents_limit).toBe("35");
    expect(params.people_limit).toBe("40");
    expect(params.q).toBeUndefined();
  });

  it("includes trimmed search text and skip_ranges when set", () => {
    const params = buildInsightsQueryParams({ query: "  alpha  ", skipRanges: true });
    expect(params.q).toBe("alpha");
    expect(params.skip_ranges).toBe("true");
  });
});

// The window the charts and header describe comes from the server, because the
// rollup-backed path reports windows ending at the last complete day rather
// than at today. Recomputing it locally drew a trailing bucket the data could
// never fill.
describe("resolveInsightsDateLabel", () => {
  it("labels the window the server reported", () => {
    expect(
      resolveInsightsDateLabel({ range: "7d", from: "2026-06-02", to: "2026-06-08" }, "7d"),
    ).toBe("Jun 2 – Jun 8");
  });

  // The reported window lands an effect after the chip flips. Showing the old
  // range's dates under the new chip would be actively wrong; the locally
  // inferred window is merely a day off on the rollup path.
  it("falls back to the local estimate when the reported window is for another range", () => {
    const stale = { range: "7d" as const, from: "2026-06-02", to: "2026-06-08" };
    expect(resolveInsightsDateLabel(stale, "30d")).not.toBe("Jun 2 – Jun 8");
  });

  it("falls back before any response has arrived", () => {
    expect(resolveInsightsDateLabel(null, "7d")).toMatch(/^\w{3} \d+ – \w{3} \d+$/);
  });
});

describe("insightsFreshnessNote", () => {
  it("names the coverage day when the server reported one", () => {
    expect(insightsFreshnessNote(true, "2026-08-05")).toBe(
      "Usage is totalled once a day. Showing everything through Aug 5.",
    );
  });

  // A cold account has a window but no coverage claim, so the note can't name a
  // day — it explains the lag instead.
  it("explains the daily lag when no coverage day was reported", () => {
    expect(insightsFreshnessNote(true)).toBe(
      "Usage is totalled once a day, so today's activity may not appear yet.",
    );
  });

  // The Langfuse path does include today; its staleness is the refresh cycle,
  // so the rollup wording would be wrong there even if an as_of leaked through.
  it("describes the refresh cycle on the Langfuse path", () => {
    expect(insightsFreshnessNote(false, "2026-08-05")).toBe(
      "Updated results may take up to 6 hours to reflect on this page.",
    );
  });
});
