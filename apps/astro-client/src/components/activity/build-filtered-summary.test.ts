import { describe, it, expect } from "vitest";
import { buildFilteredSummary, ALL_AGENTS_KEY } from "./use-insights-data";

// ALL_AGENTS_KEY is a module constant — just verify it's exported and is a string
describe("ALL_AGENTS_KEY", () => {
  it("is a non-empty string", () => {
    expect(typeof ALL_AGENTS_KEY).toBe("string");
    expect(ALL_AGENTS_KEY.length).toBeGreaterThan(0);
  });
});

type Blueprint = {
  agent_name: string;
  cost_usd: number;
  requests: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  tok_per_request: number;
  p95_latency_ms: number;
  top_model: string;
  cost_per_request: number;
  cost_over_time?: { date: string; cost_usd: number }[];
  requests_over_time?: { date: string; requests: number }[];
  tokens_over_time?: { date: string; input_tokens: number; output_tokens: number; total_tokens: number }[];
};

const period = { start: "2026-01-01T00:00:00Z", end: "2026-01-07T00:00:00Z", days: 7 };

function makeBlueprint(overrides: Partial<Blueprint> = {}): Blueprint {
  return {
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
    ...overrides,
  };
}

describe("buildFilteredSummary", () => {
  it("returns zero totals and empty cost_over_time for empty blueprints", () => {
    const result = buildFilteredSummary([], period);
    expect(result.totals.cost_usd).toBe(0);
    expect(result.totals.requests).toBe(0);
    expect(result.totals.input_tokens).toBe(0);
    expect(result.totals.output_tokens).toBe(0);
    expect(result.cost_over_time).toHaveLength(0);
  });

  it("returns zero sparklines for empty blueprints", () => {
    const result = buildFilteredSummary([], period);
    expect(result.sparklines!.cost).toHaveLength(0);
    expect(result.sparklines!.requests).toHaveLength(0);
    expect(result.sparklines!.tokens).toHaveLength(0);
  });

  it("does not throw on the all-time range where period.start/end are empty", () => {
    // /insights?range=all → server omits period.start / period.end. Date math
    // on empty strings yields NaN and naive `new Date(NaN).toISOString()`
    // throws "Invalid time value" — guard against that.
    const allTimePeriod = { start: "", end: "", days: 365 };
    const bp = makeBlueprint({ cost_over_time: [{ date: "2026-01-01", cost_usd: 1 }] });
    expect(() => buildFilteredSummary([bp], allTimePeriod)).not.toThrow();
    const result = buildFilteredSummary([bp], allTimePeriod);
    // No meaningful midpoint → change% is null across the board.
    expect(result.change?.cost_pct).toBeNull();
  });

  it("totals match single blueprint fields", () => {
    const bp = makeBlueprint({
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

  it("cost_pct is null when no activity before calendar midpoint", () => {
    // period Jan 1-7, midDate = Jan 4
    // agent only active on Jan 5+6 → prev (< Jan 4) sum = 0 → pct = null
    const bp = makeBlueprint({
      cost_over_time: [
        { date: "2026-01-05", cost_usd: 5 },
        { date: "2026-01-06", cost_usd: 5 },
      ],
    });
    const result = buildFilteredSummary([bp], period);
    expect(result.change?.cost_pct).toBeNull();
  });

  it("cost_pct is a number when previous calendar half has non-zero cost", () => {
    // period Jan 1-7, midDate = Jan 4
    // prev (< Jan 4): Jan 1 = 5 → sum = 5
    // curr (>= Jan 4): Jan 4+5 = 5+5 = 10 → pct = ((10-5)/5)*100 = 100
    const bp = makeBlueprint({
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 5 },
        { date: "2026-01-04", cost_usd: 5 },
        { date: "2026-01-05", cost_usd: 5 },
      ],
    });
    const result = buildFilteredSummary([bp], period);
    expect(result.change?.cost_pct).not.toBeNull();
    expect(typeof result.change?.cost_pct).toBe("number");
    expect(result.change?.cost_pct).toBeCloseTo(100, 0);
  });

  it("splits by calendar midpoint — prev=[Jan 1-3], curr=[Jan 4-7]", () => {
    // period Jan 1-7, midDate = Jan 4
    // prev (< Jan 4): Jan 1 = 4 → sum = 4
    // curr (>= Jan 4): Jan 4 = 16 → pct = ((16-4)/4)*100 = 300
    const bp = makeBlueprint({
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 4 },
        { date: "2026-01-04", cost_usd: 16 },
      ],
    });
    const result = buildFilteredSummary([bp], period);
    expect(result.change?.cost_pct).toBeCloseTo(300, 0);
  });

  it("sparklines.cost length equals number of distinct dates", () => {
    const bp = makeBlueprint({
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 1 },
        { date: "2026-01-02", cost_usd: 2 },
        { date: "2026-01-03", cost_usd: 3 },
        { date: "2026-01-04", cost_usd: 4 },
        { date: "2026-01-05", cost_usd: 5 },
      ],
    });
    const result = buildFilteredSummary([bp], period);
    expect(result.sparklines!.cost).toHaveLength(5);
  });

  it("sums costs from different blueprints sharing the same date", () => {
    const bpA = makeBlueprint({
      agent_name: "agent-a",
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 3 },
        { date: "2026-01-02", cost_usd: 7 },
      ],
    });
    const bpB = makeBlueprint({
      agent_name: "agent-b",
      cost_over_time: [
        { date: "2026-01-01", cost_usd: 2 },
        { date: "2026-01-02", cost_usd: 8 },
      ],
    });
    const result = buildFilteredSummary([bpA, bpB], period);
    // Dates union = ["2026-01-01", "2026-01-02"]
    expect(result.sparklines!.cost).toHaveLength(2);
    expect(result.sparklines!.cost[0]).toBeCloseTo(5, 5); // 3+2
    expect(result.sparklines!.cost[1]).toBeCloseTo(15, 5); // 7+8
  });

  it("date union spans blueprints with non-overlapping dates", () => {
    const bpA = makeBlueprint({
      agent_name: "agent-a",
      cost_over_time: [{ date: "2026-01-01", cost_usd: 10 }],
    });
    const bpB = makeBlueprint({
      agent_name: "agent-b",
      cost_over_time: [{ date: "2026-01-02", cost_usd: 20 }],
    });
    const result = buildFilteredSummary([bpA, bpB], period);
    expect(result.cost_over_time).toHaveLength(2);
  });
});
