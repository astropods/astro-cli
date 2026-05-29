import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import {
  buildFilteredSummary,
  sliceDeploymentByWindow,
  sliceDeploymentsResponseByRange,
} from "./use-insights-data";

// `buildPeriodParams` (used by `sliceDeploymentsResponseByRange`) derives the
// window from `new Date()`. Freeze it so 7d = [2026-05-20, 2026-05-26] and
// 30d = [2026-04-27, 2026-05-26], matching the fixture dates below.
beforeAll(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-05-26T12:00:00Z"));
});
afterAll(() => {
  vi.useRealTimers();
});

type Deployment = {
  deployment_id: string;
  agent_name: string;
  display_name?: string;
  namespace?: string;
  cost_usd: number;
  requests: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  tok_per_request: number;
  p95_latency_ms: number;
  top_model: string;
  cost_per_request: number;
  users_used: string[];
  cost_over_time?: { date: string; cost_usd: number }[];
  requests_over_time?: { date: string; requests: number }[];
  tokens_over_time?: { date: string; input_tokens: number; output_tokens: number; total_tokens: number }[];
};

const period = { start: "2026-01-01T00:00:00Z", end: "2026-01-07T00:00:00Z", days: 7 };

function makeDeployment(overrides: Partial<Deployment> = {}): Deployment {
  return {
    deployment_id: "dep-a",
    agent_name: "agent-a",
    cost_usd: 10,
    requests: 5,
    input_tokens: 100,
    output_tokens: 50,
    total_tokens: 150,
    tok_per_request: 30,
    p95_latency_ms: 300,
    top_model: "",
    cost_per_request: 2,
    users_used: [],
    ...overrides,
  };
}

describe("buildFilteredSummary", () => {
  it("returns zero totals for empty blueprints; cost_over_time still spans the period", () => {
    const result = buildFilteredSummary([], period);
    expect(result.totals.cost_usd).toBe(0);
    expect(result.totals.requests).toBe(0);
    expect(result.totals.input_tokens).toBe(0);
    expect(result.totals.output_tokens).toBe(0);
    // Bounded period (Jan 1–7) enumerates 7 zero-filled days regardless of
    // whether any blueprint had activity.
    expect(result.cost_over_time).toHaveLength(7);
    expect(result.cost_over_time.every((e) => e.models.length === 0)).toBe(true);
  });

  it("returns zero-filled sparklines spanning the full bounded period for empty blueprints", () => {
    const result = buildFilteredSummary([], period);
    expect(result.sparklines!.cost).toHaveLength(7);
    expect(result.sparklines!.requests).toHaveLength(7);
    expect(result.sparklines!.tokens).toHaveLength(7);
    expect(result.sparklines!.cost.every((v) => v === 0)).toBe(true);
  });

  it("enumerates the full bounded period — cost_over_time has one entry per day even if blueprints only had activity on a subset", () => {
    // Bug: /insights?range=30d would show the first bar at whichever day
    // activity began (e.g. May 7), not at the start of the requested window
    // (April 27). cost_over_time must span every day in [period.start, end].
    const bp = makeDeployment({
      cost_over_time: [
        // Only one active day, in the middle of the period.
        { date: "2026-01-04", cost_usd: 5 },
      ],
    });
    const result = buildFilteredSummary([bp], period);
    // Period is Jan 1 – Jan 7 inclusive → 7 entries.
    expect(result.cost_over_time).toHaveLength(7);
    expect(result.cost_over_time[0].date).toBe("2026-01-01");
    expect(result.cost_over_time[6].date).toBe("2026-01-07");
    // Sparkline arrays must match the same axis length.
    expect(result.sparklines!.cost).toHaveLength(7);
    expect(result.sparklines!.requests).toHaveLength(7);
    expect(result.sparklines!.tokens).toHaveLength(7);
    // Inactive days are zero-filled.
    expect(result.sparklines!.cost[0]).toBe(0);
    expect(result.sparklines!.cost[3]).toBeCloseTo(5, 5);
  });

  it("does not throw on the all-time range where period.start/end are empty", () => {
    // /insights?range=all → server omits period.start / period.end. Date math
    // on empty strings yields NaN and naive `new Date(NaN).toISOString()`
    // throws "Invalid time value" — guard against that.
    const allTimePeriod = { start: "", end: "", days: 365 };
    const bp = makeDeployment({ cost_over_time: [{ date: "2026-01-01", cost_usd: 1 }] });
    expect(() => buildFilteredSummary([bp], allTimePeriod)).not.toThrow();
    const result = buildFilteredSummary([bp], allTimePeriod);
    // No meaningful midpoint → change itself is null (matches the
    // recomputeTotalsFromUsers contract).
    expect(result.change).toBeNull();
  });

  it("totals match single blueprint fields", () => {
    const bp = makeDeployment({
      agent_name: "solo",
      cost_usd: 42.5,
      requests: 100,
      input_tokens: 2000,
      output_tokens: 500,
    });
    const result = buildFilteredSummary([bp], period);
    expect(result.totals.cost_usd).toBe(42.5);
    expect(result.totals.requests).toBe(100);
    expect(result.totals.input_tokens).toBe(2000);
    expect(result.totals.output_tokens).toBe(500);
    expect(result.totals.active_agents).toBe(1);
  });

  it("change is null when no prior totals are supplied", () => {
    const bp = makeDeployment({ cost_usd: 10, requests: 5 });
    const result = buildFilteredSummary([bp], period);
    expect(result.change).toBeNull();
  });

  it("cost_pct is null when the prior total is 0 (division by zero guard)", () => {
    const bp = makeDeployment({ cost_usd: 10, requests: 5 });
    const result = buildFilteredSummary([bp], period, { cost: 0, requests: 0, tokens: 0 });
    expect(result.change?.cost_pct).toBeNull();
    expect(result.change?.requests_pct).toBeNull();
    expect(result.change?.tokens_pct).toBeNull();
  });

  it("change% compares supplied prior totals against current totals", () => {
    // Current totals: cost 10, requests 5, tokens 150 (input 100 + output 50).
    // Prior totals: cost 5, requests 2, tokens 75 → pct ≈ {100, 150, 100}.
    const bp = makeDeployment({
      cost_usd: 10,
      requests: 5,
      input_tokens: 100,
      output_tokens: 50,
      total_tokens: 150,
    });
    const result = buildFilteredSummary([bp], period, { cost: 5, requests: 2, tokens: 75 });
    expect(result.change?.cost_pct).toBeCloseTo(100, 0);
    expect(result.change?.requests_pct).toBeCloseTo(150, 0);
    expect(result.change?.tokens_pct).toBeCloseTo(100, 0);
  });

  it("sparklines.cost length equals the bounded period length, not the number of active days", () => {
    const bp = makeDeployment({
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 1 },
        { date: "2026-01-02", cost_usd: 2 },
        { date: "2026-01-03", cost_usd: 3 },
        { date: "2026-01-04", cost_usd: 4 },
        { date: "2026-01-05", cost_usd: 5 },
      ],
    });
    const result = buildFilteredSummary([bp], period);
    // Period spans 7 days (Jan 1–7); the last two days are zero-filled.
    expect(result.sparklines!.cost).toHaveLength(7);
    expect(result.sparklines!.cost.slice(0, 5)).toEqual([1, 2, 3, 4, 5]);
    expect(result.sparklines!.cost.slice(5)).toEqual([0, 0]);
  });

  it("sums costs from different blueprints sharing the same date — full period is enumerated", () => {
    const bpA = makeDeployment({
      agent_name: "agent-a",
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 3 },
        { date: "2026-01-02", cost_usd: 7 },
      ],
    });
    const bpB = makeDeployment({
      agent_name: "agent-b",
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 2 },
        { date: "2026-01-02", cost_usd: 8 },
      ],
    });
    const result = buildFilteredSummary([bpA, bpB], period);
    // Period is Jan 1–7 → sparkline length 7; values for Jan 1+2 are merged.
    expect(result.sparklines!.cost).toHaveLength(7);
    expect(result.sparklines!.cost[0]).toBeCloseTo(5, 5); // 3+2
    expect(result.sparklines!.cost[1]).toBeCloseTo(15, 5); // 7+8
    expect(result.sparklines!.cost.slice(2)).toEqual([0, 0, 0, 0, 0]);
  });

  it("buildFilteredSummary sums sliced cost from a single blueprint correctly", () => {
    // The slicer in use-insights-data returns blueprints already trimmed to
    // the URL window. buildFilteredSummary then aggregates them — confirm
    // the rolled-up totals reflect only the sliced data, not what the
    // blueprint's all-time fields might have been.
    const bp = makeDeployment({
      // Imagine the unsliced blueprint had $100 total; this is post-slice.
      cost_usd: 8,
      requests: 4,
      input_tokens: 80,
      output_tokens: 20,
      total_tokens: 100,
      cost_over_time: [
        { date: "2026-01-02", cost_usd: 3 },
        { date: "2026-01-05", cost_usd: 5 },
      ],
    });
    const result = buildFilteredSummary([bp], period);
    expect(result.totals.cost_usd).toBe(8);
    expect(result.totals.requests).toBe(4);
    expect(result.totals.total_tokens).toBe(100);
  });

  it("sliceDeploymentByWindow trims over_time arrays and recomputes totals", () => {
    // Blueprint with activity spread across 5 days. Slicing to a 3-day window
    // should drop the out-of-window days and rebuild cost/requests/tokens.
    const all: Parameters<typeof sliceDeploymentByWindow>[0] = {
      deployment_id: "dep-alpha",
      agent_name: "alpha",
      requests: 1000, // server's all-time total — slicer should overwrite
      cost_usd: 100,
      cost_per_request: 0.1,
      input_tokens: 5000,
      output_tokens: 1000,
      total_tokens: 6000,
      tok_per_request: 6,
      p95_latency_ms: 800,
      top_model: "claude-sonnet",
      cost_over_time: [
        { date: "2026-05-20", cost_usd: 10 },
        { date: "2026-05-22", cost_usd: 20 },
        { date: "2026-05-24", cost_usd: 30 },
        { date: "2026-05-26", cost_usd: 40 },
      ],
      requests_over_time: [
        { date: "2026-05-20", requests: 100 },
        { date: "2026-05-22", requests: 200 },
        { date: "2026-05-24", requests: 300 },
        { date: "2026-05-26", requests: 400 },
      ],
      tokens_over_time: [
        { date: "2026-05-20", input_tokens: 500, output_tokens: 100, total_tokens: 600 },
        { date: "2026-05-22", input_tokens: 1000, output_tokens: 200, total_tokens: 1200 },
        { date: "2026-05-24", input_tokens: 1500, output_tokens: 300, total_tokens: 1800 },
        { date: "2026-05-26", input_tokens: 2000, output_tokens: 400, total_tokens: 2400 },
      ],
      users_used: [],
    };
    // Slice to 5-24 .. 5-26.
    const sliced = sliceDeploymentByWindow(all, "2026-05-24", "2026-05-26");
    expect(sliced.cost_over_time).toHaveLength(2);
    expect(sliced.cost_usd).toBeCloseTo(70, 5); // 30+40
    expect(sliced.requests).toBe(700); // 300+400
    expect(sliced.total_tokens).toBe(4200); // 1800+2400
    expect(sliced.cost_per_request).toBeCloseTo(70 / 700, 5);
    // p95 + top_model stay at the all-time server values (documented degradation).
    expect(sliced.p95_latency_ms).toBe(800);
    expect(sliced.top_model).toBe("claude-sonnet");
  });

  it("sliceDeploymentsResponseByRange emits different per-blueprint totals for 7d vs 30d", () => {
    // Same blueprint, sliced by two different ranges. Today is 2026-05-26
    // (per the session reminder); buildPeriodParams handles the date math.
    const allTime: Parameters<typeof sliceDeploymentsResponseByRange>[0] = {
      deployments: [{
        deployment_id: "dep-alpha",
        agent_name: "alpha",
        requests: 0,
        cost_usd: 0,
        cost_per_request: 0,
        input_tokens: 0,
        output_tokens: 0,
        total_tokens: 0,
        tok_per_request: 0,
        p95_latency_ms: 0,
        top_model: "",
        cost_over_time: [
          { date: "2026-04-27", cost_usd: 1 }, // first day of 30d range
          { date: "2026-05-10", cost_usd: 1 }, // inside 30d, outside 7d
          { date: "2026-05-21", cost_usd: 1 }, // inside 7d (5-20 .. 5-26)
        ],
        requests_over_time: [
          { date: "2026-04-27", requests: 1 },
          { date: "2026-05-10", requests: 1 },
          { date: "2026-05-21", requests: 1 },
        ],
        tokens_over_time: [
          { date: "2026-04-27", input_tokens: 0, output_tokens: 0, total_tokens: 1 },
          { date: "2026-05-10", input_tokens: 0, output_tokens: 0, total_tokens: 1 },
          { date: "2026-05-21", input_tokens: 0, output_tokens: 0, total_tokens: 1 },
        ],
        users_used: [],
      }],
      period: { start: "", end: "", days: 0 },
    };

    const sliced7d = sliceDeploymentsResponseByRange(allTime, "7d");
    const sliced30d = sliceDeploymentsResponseByRange(allTime, "30d");
    expect(sliced7d!.deployments[0].requests).toBe(1); // only 2026-05-21
    expect(sliced30d!.deployments[0].requests).toBe(3); // all three days
    expect(sliced7d!.deployments[0].cost_usd).toBeCloseTo(1, 5);
    expect(sliced30d!.deployments[0].cost_usd).toBeCloseTo(3, 5);
  });

  it("cost_over_time spans the full period — non-overlapping blueprint dates are filled, not unioned", () => {
    const bpA = makeDeployment({
      agent_name: "agent-a",
      cost_over_time: [{ date: "2026-01-01", cost_usd: 10 }],
    });
    const bpB = makeDeployment({
      agent_name: "agent-b",
      cost_over_time: [{ date: "2026-01-02", cost_usd: 20 }],
    });
    const result = buildFilteredSummary([bpA, bpB], period);
    expect(result.cost_over_time).toHaveLength(7);
    expect(result.cost_over_time[0].date).toBe("2026-01-01");
    expect(result.cost_over_time[6].date).toBe("2026-01-07");
  });
});
