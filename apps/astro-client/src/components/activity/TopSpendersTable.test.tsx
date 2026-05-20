import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TopSpendersTable } from "./TopSpendersTable";

afterEach(cleanup);

type Blueprint = {
  agent_name: string;
  cost_usd: number;
  requests: number;
  cost_per_request: number;
  tok_per_request: number;
  p95_latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  top_model: string;
};

function makeBlueprint(overrides: Partial<Blueprint> & { agent_name: string }): Blueprint {
  return {
    cost_usd: 10,
    requests: 5,
    cost_per_request: 2,
    tok_per_request: 100,
    p95_latency_ms: 300,
    input_tokens: 50,
    output_tokens: 50,
    top_model: "",
    ...overrides,
  };
}

const sampleBlueprints: Blueprint[] = [
  makeBlueprint({ agent_name: "alpha", cost_usd: 30, requests: 10 }),
  makeBlueprint({ agent_name: "beta",  cost_usd: 10, requests: 50 }),
  makeBlueprint({ agent_name: "gamma", cost_usd: 20, requests: 30 }),
];

describe("TopSpendersTable", () => {
  it("shows empty state message when blueprints is empty", () => {
    render(<TopSpendersTable blueprints={[]} />);
    expect(screen.getByText("No agent activity in this period")).toBeInTheDocument();
  });

  it("renders each blueprint's agent_name", () => {
    render(<TopSpendersTable blueprints={sampleBlueprints} />);
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("beta")).toBeInTheDocument();
    expect(screen.getByText("gamma")).toBeInTheDocument();
  });

  it("clicking 'Spend' header sorts by cost_usd descending by default; clicking again reverses to ascending", () => {
    render(<TopSpendersTable blueprints={sampleBlueprints} />);

    const spendHeader = screen.getByText("Spend");

    // First click: sort by cost_usd descending (default when switching to a new key)
    // The table already defaults to cost_usd desc, so click once to go asc
    fireEvent.click(spendHeader);

    let rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    const firstClickOrder = rows.map((r) => r.textContent);

    // Click again to reverse
    fireEvent.click(spendHeader);

    rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    const secondClickOrder = rows.map((r) => r.textContent);

    // The two orders should be reversed relative to each other
    expect(firstClickOrder).toEqual([...secondClickOrder].reverse());
  });

  it("initial sort is cost_usd descending — alpha(30) first, beta(10) last", () => {
    render(<TopSpendersTable blueprints={sampleBlueprints} />);
    const rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    expect(rows[0].textContent).toBe("alpha");
    expect(rows[rows.length - 1].textContent).toBe("beta");
  });

  it("groupLabel column header ('Agent') has no sort icon (no ↕, ↑, or ↓)", () => {
    render(<TopSpendersTable blueprints={sampleBlueprints} />);
    const agentHeader = screen.getByRole("columnheader", { name: /^Agent$/ });
    expect(agentHeader.textContent).not.toContain("↕");
    expect(agentHeader.textContent).not.toContain("↑");
    expect(agentHeader.textContent).not.toContain("↓");
  });

  it("respects custom groupLabel prop", () => {
    render(<TopSpendersTable blueprints={sampleBlueprints} groupLabel="Model" />);
    expect(screen.getByRole("columnheader", { name: /^Model$/ })).toBeInTheDocument();
  });
});
