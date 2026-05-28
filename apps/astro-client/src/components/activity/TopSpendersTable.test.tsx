import { screen, cleanup, fireEvent, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TopSpendersTable } from "./TopSpendersTable";
import { renderWithProviders } from "@/test/test-utils";
import { accountKeys } from "@/api/queries/keys";

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
  total_tokens: number;
  top_model: string;
  users_used: string[];
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
    total_tokens: 100,
    top_model: "",
    users_used: [],
    ...overrides,
  };
}

const sampleBlueprints: Blueprint[] = [
  makeBlueprint({ agent_name: "alpha", cost_usd: 30, requests: 10 }),
  makeBlueprint({ agent_name: "beta",  cost_usd: 10, requests: 50 }),
  makeBlueprint({ agent_name: "gamma", cost_usd: 20, requests: 30 }),
];

describe("TopSpendersTable", () => {
  it("shows ghost (skeleton) rows when loading=true", () => {
    const { container } = renderWithProviders(
      <TopSpendersTable mode="agents" blueprints={[]} loading={true} />
    );
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  it("shows empty state message when blueprints is empty and not loading", () => {
    renderWithProviders(<TopSpendersTable mode="agents" blueprints={[]} loading={false} />);
    expect(screen.getByText("No agent activity in this period")).toBeInTheDocument();
  });

  it("renders each blueprint's agent_name", () => {
    renderWithProviders(<TopSpendersTable mode="agents" blueprints={sampleBlueprints} loading={false} />);
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("beta")).toBeInTheDocument();
    expect(screen.getByText("gamma")).toBeInTheDocument();
  });

  it("clicking 'Total Spend' header sorts by cost_usd descending by default; clicking again reverses to ascending", () => {
    renderWithProviders(<TopSpendersTable mode="agents" blueprints={sampleBlueprints} loading={false} />);

    const spendHeader = screen.getByText("Total Spend");

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
    renderWithProviders(<TopSpendersTable mode="agents" blueprints={sampleBlueprints} loading={false} />);
    const rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    expect(rows[0].textContent).toBe("alpha");
    expect(rows[rows.length - 1].textContent).toBe("beta");
  });

  it("groupLabel column header ('Agent') has no sort icon (no ↕, ↑, or ↓)", () => {
    renderWithProviders(<TopSpendersTable mode="agents" blueprints={sampleBlueprints} loading={false} />);
    const agentHeader = screen.getByRole("columnheader", { name: /^Agent$/ });
    expect(agentHeader.textContent).not.toContain("↕");
    expect(agentHeader.textContent).not.toContain("↑");
    expect(agentHeader.textContent).not.toContain("↓");
  });

  it("respects custom groupLabel prop", () => {
    renderWithProviders(
      <TopSpendersTable mode="agents" blueprints={sampleBlueprints} loading={false} groupLabel="Model" />
    );
    expect(screen.getByRole("columnheader", { name: /^Model$/ })).toBeInTheDocument();
  });

  it("renders a Users column; empty users_used renders the em-dash", () => {
    const blueprints: Blueprint[] = [
      makeBlueprint({ agent_name: "alpha", cost_usd: 30, users_used: ["u_alice", "u_bob", "u_carol"] }),
      makeBlueprint({ agent_name: "beta",  cost_usd: 10, users_used: [] }),
    ];
    renderWithProviders(<TopSpendersTable mode="agents" blueprints={blueprints} loading={false} />);
    expect(screen.getByRole("columnheader", { name: /^Users$/ })).toBeInTheDocument();
    const rows = screen.getAllByRole("row").slice(1); // skip header
    const betaRow = rows.find((r) => within(r).queryByText("beta"))!;
    // Empty users_used renders as em-dash, not "0".
    expect(within(betaRow).getByText("—")).toBeInTheDocument();
  });
});

// ── Users mode ────────────────────────────────────────────────────────────────

type UserRow = {
  user_id: string;
  cost_usd: number;
  requests: number;
  tokens: number;
  last_seen?: string;
  agents_used: Array<{ name: string; account: string }>;
};

function makeUserRow(overrides: Partial<UserRow> & { user_id: string }): UserRow {
  return {
    cost_usd: 1,
    requests: 1,
    tokens: 100,
    last_seen: "2026-05-01T00:00:00Z",
    agents_used: [],
    ...overrides,
  };
}

const ACCOUNT = "acme";

function seedMembers(qc: ReturnType<typeof renderWithProviders>["queryClient"], members: Array<{ user_id: string; username: string; display_name: string }>) {
  qc.setQueryData(accountKeys.members(ACCOUNT), {
    members: members.map((m) => ({
      account_id: "acct-acme",
      user_id: m.user_id,
      role: "member",
      status: "active",
      username: m.username,
      display_name: m.display_name,
      created_at: "2025-01-01T00:00:00Z",
      slack_workspaces: [],
    })),
  });
}

describe("TopSpendersTable users mode", () => {
  it("shows ghost rows when loading=true", () => {
    const { container } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} users={[]} loading={true} />,
    );
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  it("shows the empty-state message when users is empty and not loading", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} users={[]} loading={false} />,
    );
    // The table also waits on the members query for classification — seed
    // empty members so the loading gate clears.
    seedMembers(queryClient, []);
    expect(await screen.findByText("No user activity in this period")).toBeInTheDocument();
  });

  it("renders member display_name via UserBadge and renders AgentsUsedChips per row", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({
            user_id: "u_alice",
            cost_usd: 10,
            agents_used: [
              { name: "alpha", account: ACCOUNT },
              { name: "beta", account: ACCOUNT },
            ],
          }),
        ]}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    // findByText awaits the re-render triggered by setQueryData above.
    expect(await screen.findByText("Alice Chen")).toBeInTheDocument();
    // AgentsUsedChips renders the agent list as an sr-only label.
    expect(screen.getByText("alpha, beta")).toBeInTheDocument();
  });

  it("default-sorts by cost_usd desc for member rows", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "u_low", cost_usd: 1 }),
          makeUserRow({ user_id: "u_high", cost_usd: 10 }),
          makeUserRow({ user_id: "u_mid", cost_usd: 5 }),
        ]}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_low", username: "low", display_name: "Low Spender" },
      { user_id: "u_high", username: "high", display_name: "High Spender" },
      { user_id: "u_mid", username: "mid", display_name: "Mid Spender" },
    ]);

    // Wait for the re-render that resolves member names.
    await screen.findByText("High Spender");
    const rows = screen.getAllByRole("row");
    const bodyRowText = rows.slice(1).map((r) => r.textContent ?? "");
    const highIdx = bodyRowText.findIndex((t) => t.includes("High Spender"));
    const midIdx = bodyRowText.findIndex((t) => t.includes("Mid Spender"));
    const lowIdx = bodyRowText.findIndex((t) => t.includes("Low Spender"));
    expect(highIdx).toBeLessThan(midIdx);
    expect(midIdx).toBeLessThan(lowIdx);
  });

  it("aggregates non-member rows into an Unidentified bucket pinned to the bottom", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "u_alice", cost_usd: 5 }),
          makeUserRow({ user_id: "u_ext_1", cost_usd: 3 }),
          makeUserRow({ user_id: "u_ext_2", cost_usd: 2 }),
        ]}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    const unidentified = await screen.findByText(/Unidentified · 2 users/);
    expect(unidentified).toBeInTheDocument();

    // The Unidentified row should sit at the bottom (after Alice).
    const allRows = screen.getAllByRole("row");
    const aliceRow = allRows.find((r) => within(r).queryByText("Alice Chen"))!;
    const bucketRow = allRows.find((r) => within(r).queryByText(/Unidentified/))!;
    expect(allRows.indexOf(aliceRow)).toBeLessThan(allRows.indexOf(bucketRow));
  });

  it("aggregates empty user_id rows into the Unattributed bucket", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "", cost_usd: 0.75 }),
        ]}
      />,
    );
    seedMembers(queryClient, []);
    expect(await screen.findByText("Infrastructure")).toBeInTheDocument();
  });
});
