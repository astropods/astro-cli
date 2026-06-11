// Regression test for PR #1335. Before the per-chart hydration gate, recharts
// rendered its ResponsiveContainer during SSR, got width(-1) height(-1), and
// threw — aborting the /insights stream on hard refresh. These tests assert
// that with real chart data the SSR pass produces the placeholder, not the
// recharts SVG, and never throws.
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CostOverTimeChart } from "./CostOverTimeChart";
import { ActiveUsersSpendChart } from "./ActiveUsersSpendChart";

const COST_OVER_TIME_DATA = [
  { date: "2026-06-09", models: [{ model: "gpt-4", cost_usd: 12.5 }] },
  { date: "2026-06-10", models: [{ model: "gpt-4", cost_usd: 18.0 }] },
  { date: "2026-06-11", models: [{ model: "gpt-4", cost_usd: 9.25 }] },
];

const ACTIVE_USERS_DATA = [
  { date: "2026-06-09", users: 3, cost: 12.5 },
  { date: "2026-06-10", users: 5, cost: 18.0 },
  { date: "2026-06-11", users: 4, cost: 9.25 },
];

describe("Insights chart SSR safety", () => {
  it("CostOverTimeChart renders the loading placeholder on SSR with populated data", () => {
    let html = "";
    expect(() => {
      html = renderToString(<CostOverTimeChart data={COST_OVER_TIME_DATA} days={30} />);
    }).not.toThrow();

    expect(html).toContain("Loading chart...");
    // No recharts <svg> on SSR — would mean ResponsiveContainer reached
    // its child chart, which is the failure path.
    expect(html).not.toMatch(/<svg/);
  });

  it("ActiveUsersSpendChart renders the loading placeholder on SSR with populated data", () => {
    let html = "";
    expect(() => {
      html = renderToString(<ActiveUsersSpendChart data={ACTIVE_USERS_DATA} days={30} />);
    }).not.toThrow();

    expect(html).toContain("Loading chart...");
    expect(html).not.toMatch(/<svg/);
  });

  it("CostOverTimeChart still renders 'No spend yet' for empty data on SSR", () => {
    const html = renderToString(<CostOverTimeChart data={[]} days={30} />);
    expect(html).toContain("No spend yet");
  });

  it("ActiveUsersSpendChart still renders 'No spend yet' for empty data on SSR", () => {
    const html = renderToString(<ActiveUsersSpendChart data={[]} days={30} />);
    expect(html).toContain("No spend yet");
  });
});
