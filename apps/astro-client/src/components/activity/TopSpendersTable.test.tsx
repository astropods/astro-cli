import { screen, cleanup, fireEvent, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TopSpendersTable } from "./TopSpendersTable";
import { renderWithProviders } from "@/test/test-utils";
import { accountKeys } from "@/api/queries/keys";

afterEach(cleanup);

type Deployment = {
  deployment_id: string;
  agent_name: string;
  display_name?: string;
  namespace?: string;
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

function makeDeployment(overrides: Partial<Deployment> & { agent_name: string }): Deployment {
  return {
    deployment_id: `dep-${overrides.agent_name}`,
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

const sampleDeployments: Deployment[] = [
  makeDeployment({ agent_name: "alpha", cost_usd: 30, requests: 10 }),
  makeDeployment({ agent_name: "beta",  cost_usd: 10, requests: 50 }),
  makeDeployment({ agent_name: "gamma", cost_usd: 20, requests: 30 }),
];

describe("TopSpendersTable (agents view, per-deployment rows)", () => {
  it("shows ghost (skeleton) rows when loading=true", () => {
    const { container } = renderWithProviders(
      <TopSpendersTable mode="agents" deployments={[]} loading={true} />
    );
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  it("shows empty state message when deployments is empty and not loading", () => {
    renderWithProviders(<TopSpendersTable mode="agents" deployments={[]} loading={false} />);
    expect(screen.getByText("No deployment activity in this period")).toBeInTheDocument();
  });

  it("renders each deployment's agent_name as the row label", () => {
    renderWithProviders(<TopSpendersTable mode="agents" deployments={sampleDeployments} loading={false} />);
    expect(screen.getByText("alpha")).toBeInTheDocument();
    expect(screen.getByText("beta")).toBeInTheDocument();
    expect(screen.getByText("gamma")).toBeInTheDocument();
  });

  it("clicking 'Total Spend' header sorts by cost_usd descending by default; clicking again reverses to ascending", () => {
    renderWithProviders(<TopSpendersTable mode="agents" deployments={sampleDeployments} loading={false} />);

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
    renderWithProviders(<TopSpendersTable mode="agents" deployments={sampleDeployments} loading={false} />);
    const rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    expect(rows[0].textContent).toBe("alpha");
    expect(rows[rows.length - 1].textContent).toBe("beta");
  });

  it("groupLabel column header ('Name') has no sort icon (no ↕, ↑, or ↓)", () => {
    renderWithProviders(<TopSpendersTable mode="agents" deployments={sampleDeployments} loading={false} />);
    const header = screen.getByRole("columnheader", { name: /^Name$/ });
    expect(header.textContent).not.toContain("↕");
    expect(header.textContent).not.toContain("↑");
    expect(header.textContent).not.toContain("↓");
  });

  it("respects custom groupLabel prop", () => {
    renderWithProviders(
      <TopSpendersTable mode="agents" deployments={sampleDeployments} loading={false} groupLabel="Model" />
    );
    expect(screen.getByRole("columnheader", { name: /^Model$/ })).toBeInTheDocument();
  });

  it("renders a People column; empty users_used renders the em-dash", () => {
    const deployments: Deployment[] = [
      makeDeployment({ agent_name: "alpha", cost_usd: 30, users_used: ["u_alice", "u_bob", "u_carol"] }),
      makeDeployment({ agent_name: "beta",  cost_usd: 10, users_used: [] }),
    ];
    renderWithProviders(<TopSpendersTable mode="agents" deployments={deployments} loading={false} />);
    expect(screen.getByRole("columnheader", { name: /^People$/ })).toBeInTheDocument();
    const rows = screen.getAllByRole("row").slice(1); // skip header
    const betaRow = rows.find((r) => within(r).queryByText("beta"))!;
    // Empty users_used renders as em-dash, not "0".
    expect(within(betaRow).getByText("—")).toBeInTheDocument();
  });

  it("two deployments of the same agent_name render as separate rows (no rollup)", () => {
    const deployments: Deployment[] = [
      makeDeployment({ deployment_id: "dep-east", agent_name: "swipefile", display_name: "Swipefile East", cost_usd: 30 }),
      makeDeployment({ deployment_id: "dep-west", agent_name: "swipefile", display_name: "Swipefile West", cost_usd: 10 }),
    ];
    renderWithProviders(<TopSpendersTable mode="agents" deployments={deployments} loading={false} />);
    // Both display names visible → both rows rendered (no agent_name rollup).
    expect(screen.getByText("Swipefile East")).toBeInTheDocument();
    expect(screen.getByText("Swipefile West")).toBeInTheDocument();
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
  // Mirrors AccountUsersSummaryResponse.users — keep this field in the
  // local type so the deep-link assertions below would fail to compile
  // if the API ever dropped `slack_team_id`. The test exercises it via
  // makeUserRow({ slack_team_id: "T07XYZ" }).
  slack_team_id?: string;
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
    expect(await screen.findByText("No activity from people in this period")).toBeInTheDocument();
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

    const unidentified = await screen.findByText(/Unidentified · 2 people/);
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
    expect(await screen.findByText("System spend")).toBeInTheDocument();
  });

  // Named members and unlinked Slack users sort into one merged list by
  // spend — a Slack user out-spending a named member shows up above them.
  // The row label component differentiates them visually (UserBadge vs
  // SlackUserIdentity), but ordering is on cost alone.
  it("sorts Slack users alongside named members by spend (merged, not grouped)", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "u_alice", cost_usd: 1 }),
          makeUserRow({ user_id: "U07HEAVY1", cost_usd: 9 }),
        ]}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    // Wait for member name to resolve so we can assert row order.
    await screen.findByText("Alice Chen");
    const allRows = screen.getAllByRole("row");
    const slackRow = allRows.find((r) => within(r).queryByText("Slack user - U07HEAVY1"))!;
    const aliceRow = allRows.find((r) => within(r).queryByText("Alice Chen"))!;
    // Higher-spend Slack user must sort above lower-spend named member —
    // proves the two groups aren't rendered as separate blocks.
    expect(allRows.indexOf(slackRow)).toBeLessThan(allRows.indexOf(aliceRow));
  });

  // Admins clicking a Slack row should be punted into Slack's user-profile
  // UI so they can see who the human behind the id is. The deep link uses
  // team_id from the server-side directory join (slack_team_id on the
  // response), not from parsing user_id.
  it("renders a slack:// deep-link anchor when slack_team_id is present", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "U07ABCDEF", cost_usd: 4, slack_team_id: "T07XYZ" }),
        ]}
      />,
    );
    seedMembers(queryClient, []);

    const label = await screen.findByText("Slack user - U07ABCDEF");
    const anchor = label.closest("a");
    expect(anchor).not.toBeNull();
    expect(anchor!.getAttribute("href")).toBe("slack://user?team=T07XYZ&id=U07ABCDEF");
  });

  // When the directory join misses (e.g. tombstoned user, pre-backfill), the
  // server omits slack_team_id and the row must stay plain text — clicking
  // an empty-team deep link would land on a broken Slack URL.
  it("renders plain text (no anchor) when slack_team_id is missing", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "U01LEGACY", cost_usd: 1 }),
        ]}
      />,
    );
    seedMembers(queryClient, []);

    const label = await screen.findByText("Slack user - U01LEGACY");
    expect(label.closest("a")).toBeNull();
  });

  // Unlinked Slack users used to disappear into the Unidentified bucket
  // alongside any other non-member id. Now each one renders as its own
  // "Slack user - U07…" row so a CEO scanning Insights can see which
  // workspace teammates are driving spend without first asking them to
  // link an Astro account.
  it("renders unlinked Slack users as per-id rows, not aggregated", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "U07ABCDEF", cost_usd: 4, slack_team_id: "T07XYZ" }),
          makeUserRow({ user_id: "U01LEGACY", cost_usd: 2 }),
          makeUserRow({ user_id: "u_alice", cost_usd: 6 }),
        ]}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    expect(await screen.findByText("Alice Chen")).toBeInTheDocument();
    expect(screen.getByText("Slack user - U07ABCDEF")).toBeInTheDocument();
    expect(screen.getByText("Slack user - U01LEGACY")).toBeInTheDocument();
    // The aggregated Unidentified bucket must NOT appear for slack ids.
    expect(screen.queryByText(/Unidentified · /)).not.toBeInTheDocument();
  });
});
