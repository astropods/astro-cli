import { describe, it, expect, beforeAll, afterAll, vi } from "vitest";
import {
  sliceUsersByRange,
  shiftPriorWindow,
} from "./use-insights-data";

// `rangeWindow` (used by sliceUsersByRange for both per-user totals and
// per-day sparklines) derives the window from `new Date()`. Freeze it so the
// 7d range is a stable, known window: [2026-01-04, 2026-01-10] inclusive.
beforeAll(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-01-10T12:00:00Z"));
});
afterAll(() => {
  vi.useRealTimers();
});

// ── Fixture helpers ─────────────────────────────────────────────────────────

type CostOverTimeByUser = NonNullable<
  Parameters<typeof sliceUsersByRange>[0]
>["cost_over_time_by_user"];

function summary(rows: CostOverTimeByUser): Parameters<typeof sliceUsersByRange>[0] {
  return {
    period: { start: "", end: "", days: 0 },
    totals: { cost_usd: 0, requests: 0, input_tokens: 0, output_tokens: 0, total_tokens: 0, active_agents: 0 },
    daily_avg: { cost_usd: 0, requests: 0, tokens: 0 },
    cost_over_time: [],
    cost_by_model: [],
    sparklines: { cost: [], requests: [], tokens: [] },
    cost_over_time_by_user: rows,
  };
}

function usersData(users: Parameters<typeof sliceUsersByRange>[1] extends infer U
  ? U extends { users: infer A } ? A : never
  : never): Parameters<typeof sliceUsersByRange>[1] {
  return { users, period: { start: "", end: "", days: 0 } };
}

// ── sliceUsersByRange ───────────────────────────────────────────────────────

describe("sliceUsersByRange — users output", () => {
  it("7d range excludes out-of-window days and aggregates per-user cost / requests / tokens", () => {
    const s = summary([
      // Inside 7d window (2026-01-04 .. 2026-01-10):
      { date: "2026-01-05", users: [
        { user_id: "u_alice", cost_usd: 3, requests: 30, tokens: 300 },
        { user_id: "u_bob",   cost_usd: 1, requests: 10, tokens: 100 },
      ]},
      { date: "2026-01-09", users: [
        { user_id: "u_alice", cost_usd: 2, requests: 20, tokens: 200 },
      ]},
      // Outside the 7d window — must be ignored:
      { date: "2025-12-30", users: [
        { user_id: "u_alice", cost_usd: 999, requests: 999, tokens: 999 },
      ]},
      { date: "2026-01-02", users: [
        { user_id: "u_bob", cost_usd: 999, requests: 999, tokens: 999 },
      ]},
    ]);
    const { users } = sliceUsersByRange(s, usersData([
      { user_id: "u_alice", cost_usd: 0, requests: 0, tokens: 0, agents_used: [{ name: "agent-a", account: "acme" }] },
      { user_id: "u_bob",   cost_usd: 0, requests: 0, tokens: 0, agents_used: [] },
    ]), "7d");

    const byId = new Map(users.map((u) => [u.user_id, u]));
    expect(byId.get("u_alice")?.cost_usd).toBeCloseTo(5, 5);   // 3+2
    expect(byId.get("u_alice")?.requests).toBe(50);            // 30+20
    expect(byId.get("u_alice")?.tokens).toBe(500);             // 300+200
    expect(byId.get("u_bob")?.cost_usd).toBeCloseTo(1, 5);     // only 2026-01-05
    expect(byId.get("u_bob")?.requests).toBe(10);
    // agents_used flows through from the all-time usersData payload.
    expect(byId.get("u_alice")?.agents_used[0]?.name).toBe("agent-a");
  });

  it("last_seen picks the most recent active day in the window", () => {
    const s = summary([
      { date: "2026-01-05", users: [{ user_id: "u_alice", cost_usd: 1, requests: 1, tokens: 1 }] },
      // No activity (zero cost + zero requests) on 2026-01-09 — should NOT bump last_seen.
      { date: "2026-01-09", users: [{ user_id: "u_alice", cost_usd: 0, requests: 0, tokens: 0 }] },
      { date: "2026-01-07", users: [{ user_id: "u_alice", cost_usd: 2, requests: 2, tokens: 2 }] },
    ]);
    const { users } = sliceUsersByRange(s, usersData([
      { user_id: "u_alice", cost_usd: 0, requests: 0, tokens: 0, agents_used: [] },
    ]), "7d");

    // Most recent ACTIVE day is 2026-01-07.
    expect(users[0].last_seen).toBe("2026-01-07T00:00:00Z");
  });

  it("all-time range returns the original users array verbatim", () => {
    const allTimeUsers = [
      { user_id: "u_alice", cost_usd: 100, requests: 50, tokens: 1000, agents_used: [], last_seen: "2025-08-01T00:00:00Z" },
    ];
    const s = summary([
      // Even with per-day rows present, all-time should bypass slicing.
      { date: "2026-01-05", users: [{ user_id: "u_alice", cost_usd: 1, requests: 1, tokens: 1 }] },
    ]);

    const { users } = sliceUsersByRange(s, usersData(allTimeUsers), "all");
    expect(users).toBe(allTimeUsers); // identity — no copy
  });

  it("falls back to the original users array when cost_over_time_by_user is absent", () => {
    const allTimeUsers = [
      { user_id: "u_alice", cost_usd: 100, requests: 50, tokens: 1000, agents_used: [] },
    ];
    const s = summary(undefined);

    const { users } = sliceUsersByRange(s, usersData(allTimeUsers), "7d");
    expect(users).toBe(allTimeUsers); // identity — fallback path
  });
});

// ── sliceUsersByRange — sparklines output ───────────────────────────────────

describe("sliceUsersByRange — sparklines output", () => {
  it("returns arrays of length === bounded-period days (7 for the 7d range), zero-filling inactive days", () => {
    const s = summary([
      { date: "2026-01-05", users: [{ user_id: "u_alice", cost_usd: 4, requests: 40, tokens: 400 }] },
      { date: "2026-01-08", users: [{ user_id: "u_alice", cost_usd: 6, requests: 60, tokens: 600 }] },
    ]);

    const { sparklines } = sliceUsersByRange(s, usersData([]), "7d", null, new Set());
    expect(sparklines.cost).toHaveLength(7);
    expect(sparklines.requests).toHaveLength(7);
    expect(sparklines.tokens).toHaveLength(7);
    // Window is 2026-01-04 (idx 0) .. 2026-01-10 (idx 6).
    // 2026-01-05 is idx 1 with cost=4; 2026-01-08 is idx 4 with cost=6.
    expect(sparklines.cost[1]).toBeCloseTo(4, 5);
    expect(sparklines.cost[4]).toBeCloseTo(6, 5);
    // Inactive days zero-filled.
    expect(sparklines.cost[0]).toBe(0);
    expect(sparklines.cost[2]).toBe(0);
    expect(sparklines.cost[6]).toBe(0);
  });

  it("user filter restricts which users contribute to the sums", () => {
    const memberIds = new Set(["u_alice", "u_bob"]);
    const s = summary([
      { date: "2026-01-05", users: [
        { user_id: "u_alice", cost_usd: 4, requests: 40, tokens: 400 },
        { user_id: "u_bob",   cost_usd: 6, requests: 60, tokens: 600 },
      ]},
    ]);

    // No filter (null) → both users contribute.
    const { sparklines: all } = sliceUsersByRange(s, usersData([]), "7d", null, memberIds);
    expect(all.cost[1]).toBeCloseTo(10, 5); // 4+6 on 2026-01-05

    // Filter to just alice → only her cost contributes.
    const { sparklines: onlyAlice } = sliceUsersByRange(s, usersData([]), "7d", new Set(["u_alice"]), memberIds);
    expect(onlyAlice.cost[1]).toBeCloseTo(4, 5);
  });

  it("returns empty arrays when cost_over_time_by_user is missing", () => {
    const s = summary(undefined);
    const { sparklines } = sliceUsersByRange(s, usersData([]), "7d", null, new Set());
    expect(sparklines.cost).toEqual([]);
    expect(sparklines.requests).toEqual([]);
    expect(sparklines.tokens).toEqual([]);
  });
});

// ── shiftPriorWindow ────────────────────────────────────────────────────────

describe("shiftPriorWindow", () => {
  it("returns same-length window immediately before the input", () => {
    // 7d range [Jan 4..Jan 10] → prior [Dec 28..Jan 3]
    expect(shiftPriorWindow("2026-01-04", "2026-01-10")).toEqual({
      priorFrom: "2025-12-28",
      priorTo: "2026-01-03",
    });
    // 30d range [Apr 27..May 26] → prior [Mar 28..Apr 26]
    expect(shiftPriorWindow("2026-04-27", "2026-05-26")).toEqual({
      priorFrom: "2026-03-28",
      priorTo: "2026-04-26",
    });
    // Single-day window
    expect(shiftPriorWindow("2026-01-10", "2026-01-10")).toEqual({
      priorFrom: "2026-01-09",
      priorTo: "2026-01-09",
    });
    // Malformed input → null
    expect(shiftPriorWindow("", "")).toBeNull();
    // from > to → null
    expect(shiftPriorWindow("2026-01-10", "2026-01-04")).toBeNull();
  });
});
