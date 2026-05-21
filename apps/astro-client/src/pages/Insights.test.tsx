import { describe, it, expect } from "vitest";
import { shouldRevalidate } from "./Insights";

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

  it("skips revalidation when only non-view search params change on the same pathname", () => {
    expect(shouldRevalidate(args("http://x/insights", "http://x/insights?range=7d"))).toBe(false);
    expect(shouldRevalidate(args("http://x/insights?range=7d", "http://x/insights?range=14d"))).toBe(false);
    expect(shouldRevalidate(args("http://x/insights?range=7d", "http://x/insights?range=7d&agents=a,b"))).toBe(false);
  });

  it("revalidates when the view param changes — loader needs to fetch different data", () => {
    // agents (implicit, no param) → users
    expect(shouldRevalidate(args("http://x/insights", "http://x/insights?view=users"))).toBe(true);
    // users → agents (param removed)
    expect(shouldRevalidate(args("http://x/insights?view=users", "http://x/insights"))).toBe(true);
    // view + range change together — still revalidates because view differs
    expect(shouldRevalidate(args("http://x/insights?range=7d", "http://x/insights?range=14d&view=users"))).toBe(true);
  });

  it("defers to defaultShouldRevalidate when the pathname actually changed", () => {
    expect(shouldRevalidate(args("http://x/agents", "http://x/insights", true))).toBe(true);
    expect(shouldRevalidate(args("http://x/agents", "http://x/insights", false))).toBe(false);
  });
});
