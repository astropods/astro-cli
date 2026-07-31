import { describe, it, expect } from "vitest";
import { buildInsightsQueryParams, shouldRevalidate } from "./Insights";

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
