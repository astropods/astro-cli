import { screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TopSpendersTable } from "./TopSpendersTable";
import { renderWithProviders } from "@/test/test-utils";
import { accountKeys } from "@/api/queries/keys";
import type { UserIdentity } from "@/lib/api";

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
  users_used_details?: UserIdentity[];
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

const ACCOUNT = "acme";

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

  it("clicking 'Spend' header sorts by cost_usd descending by default; clicking again reverses to ascending", () => {
    renderWithProviders(<TopSpendersTable mode="agents" deployments={sampleDeployments} loading={false} />);

    const spendHeader = screen.getByText("Spend");
    const labelOf = (cell: HTMLElement) => cell.textContent?.trim().replace(/^\d+/, "");

    // First click: sort by cost_usd descending (default when switching to a new key)
    // The table already defaults to cost_usd desc, so click once to go asc
    fireEvent.click(spendHeader);

    let rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    const firstClickOrder = rows.map(labelOf);

    // Click again to reverse
    fireEvent.click(spendHeader);

    rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    const secondClickOrder = rows.map(labelOf);

    // The two orders should be reversed relative to each other
    expect(firstClickOrder).toEqual([...secondClickOrder].reverse());
  });

  it("initial sort is cost_usd descending — alpha(30) first, beta(10) last", () => {
    renderWithProviders(<TopSpendersTable mode="agents" deployments={sampleDeployments} loading={false} />);
    const rows = screen.getAllByRole("cell", { name: /alpha|beta|gamma/ });
    const labelOf = (cell: HTMLElement) => cell.textContent?.trim().replace(/^\d+/, "");
    expect(labelOf(rows[0])).toBe("alpha");
    expect(labelOf(rows[rows.length - 1])).toBe("beta");
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

  it("renders a Used by column; empty users_used renders the em-dash", () => {
    const deployments: Deployment[] = [
      makeDeployment({ agent_name: "alpha", cost_usd: 30, users_used: ["u_alice", "u_bob", "u_carol"] }),
      makeDeployment({ agent_name: "beta",  cost_usd: 10, users_used: [] }),
    ];
    renderWithProviders(<TopSpendersTable mode="agents" deployments={deployments} loading={false} />);
    expect(screen.getByRole("columnheader", { name: /^Used by$/ })).toBeInTheDocument();
    const rows = screen.getAllByRole("row").slice(1); // skip header
    const betaRow = rows.find((r) => within(r).queryByText("beta"))!;
    // Empty users_used renders as em-dash, not "0".
    expect(within(betaRow).getByText("—")).toBeInTheDocument();
  });

  it("uses rich Slack Used by identities for names, avatars, and deep links", async () => {
    const deployments: Deployment[] = [
      makeDeployment({
        agent_name: "alpha",
        cost_usd: 30,
        users_used: ["U07SOHUM1"],
        users_used_details: [
          {
            user_id: "U07SOHUM1",
            user_details: {
              kind: "slack",
              team_id: "TPOSTMAN",
              display_name: "Sohum Dalal",
              username: "sohum",
              avatar_url: "https://avatars.slack-edge.com/sohum-postman.png",
            },
          },
          {
            user_id: "U07SOHUM1",
            user_details: {
              kind: "slack",
              team_id: "TASTRO",
              display_name: "Sohum Dalal",
              username: "sohum",
            },
          },
          { user_id: "u_alice", user_details: { kind: "astro" } },
          { user_id: "anon-user", user_details: { kind: "unknown" } },
        ],
      }),
    ];
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="agents" account={ACCOUNT} deployments={deployments} loading={false} />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    fireEvent.click(await screen.findByRole("button", { name: "Show 4 people" }));

    expect(await screen.findAllByText("sohum")).toHaveLength(2);
    expect(screen.queryByText("(Postman) - @sohum - U07SOHUM1")).not.toBeInTheDocument();
    expect(screen.queryByText("(Astro) - @sohum - U07SOHUM1")).not.toBeInTheDocument();
    expect(screen.getByText("Alice Chen")).toBeInTheDocument();
    expect(screen.getByText("anon-user")).toBeInTheDocument();

    const slackAnchors = screen.getAllByText("sohum").map((label) => label.closest("a"));
    const slackLinks = slackAnchors.map((anchor) => anchor?.getAttribute("href"));
    expect(slackLinks).toEqual(expect.arrayContaining([
      "slack://user?team=TPOSTMAN&id=U07SOHUM1",
      "slack://user?team=TASTRO&id=U07SOHUM1",
    ]));
    expect(slackAnchors.map((anchor) => anchor?.getAttribute("title"))).toContain("sohum");
    expect(screen.getAllByTitle("Alice Chen (@alice)").length).toBeGreaterThan(0);
    const avatar = document.querySelector('img[src="https://avatars.slack-edge.com/sohum-postman.png"]');
    expect(avatar).not.toBeNull();
    expect(avatar).toHaveClass("rounded-full");
    expect(avatar).toHaveClass("opacity-60");
  });

  it("prefers Used by details so linked Slack history renders as the Astro member", async () => {
    const deployments: Deployment[] = [
      makeDeployment({
        agent_name: "alpha",
        cost_usd: 30,
        users_used: ["U07SOHUM1"],
        users_used_details: [{ user_id: "u_alice", user_details: { kind: "astro" } }],
      }),
    ];
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="agents" account={ACCOUNT} deployments={deployments} loading={false} />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    const memberLink = await screen.findByRole("link", { name: "Alice Chen" });
    expect(memberLink.getAttribute("href")).toBe("/alice");
    expect(screen.queryByText(/U07SOHUM1/)).not.toBeInTheDocument();
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

  // Mirrors the users-mode "collapses long lists" test — verifies the shared
  // <TableShowMore> wiring works in agents mode too.
  it("collapses long deployment lists with a Show-more toggle that expands and re-collapses", async () => {
    const deployments: Deployment[] = Array.from({ length: 7 }, (_, i) =>
      makeDeployment({
        deployment_id: `dep-${String(i).padStart(2, "0")}`,
        agent_name: `agent-${String(i).padStart(2, "0")}`,
        display_name: `Agent ${String(i).padStart(2, "0")}`,
        cost_usd: 100 - i,
      }),
    );
    renderWithProviders(<TopSpendersTable mode="agents" deployments={deployments} loading={false} />);

    // Initial render: top 5 by cost desc visible, last 2 hidden.
    expect(screen.getByText("Agent 00")).toBeInTheDocument();
    expect(screen.getByText("Agent 04")).toBeInTheDocument();
    expect(screen.queryByText("Agent 05")).not.toBeInTheDocument();
    expect(screen.queryByText("Agent 06")).not.toBeInTheDocument();

    const showMore = screen.getByRole("button", { name: /^Show all$/ });
    fireEvent.click(showMore);

    // Expanded: all 7 visible, button toggles to "Show less".
    expect(screen.getByText("Agent 05")).toBeInTheDocument();
    expect(screen.getByText("Agent 06")).toBeInTheDocument();
    const showLess = screen.getByRole("button", { name: /^Show less$/ });
    fireEvent.click(showLess);

    // AnimatePresence keeps exiting rows mounted until exit completes.
    await waitFor(() =>
      expect(screen.queryByText("Agent 06")).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: /^Show all$/ })).toBeInTheDocument();
  });
});

// ── Users mode ────────────────────────────────────────────────────────────────

type UserRow = {
  user_id: string;
  user_details: { kind: "astro" | "slack" | "unknown"; team_id?: string; display_name?: string; username?: string; avatar_url?: string; is_bot?: boolean; deleted?: boolean };
  cost_usd: number;
  requests: number;
  tokens: number;
  last_seen?: string;
  agents_used: Array<{ deployment_id: string; name: string; account: string }>;
};

// makeUserRow defaults user_details from the user_id shape (matching
// classifyUserID server-side). Pass `user_details: {...}` to override —
// the deep-link assertions need a populated team_id, the enrichment
// assertions need display_name / avatar_url, etc.
function makeUserRow(overrides: Partial<UserRow> & { user_id: string }): UserRow {
  const inferKind = (uid: string): "astro" | "slack" | "unknown" => {
    if (/^U[A-Z0-9]{6,9}$/.test(uid)) return "slack";
    if (uid.startsWith("user_")) return "astro";
    return "unknown";
  };
  return {
    cost_usd: 1,
    requests: 1,
    tokens: 100,
    last_seen: "2026-05-01T00:00:00Z",
    agents_used: [],
    user_details: overrides.user_details ?? { kind: inferKind(overrides.user_id) },
    ...overrides,
  };
}

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
              { deployment_id: "dep-alpha", name: "alpha", account: ACCOUNT },
              { deployment_id: "dep-beta", name: "beta", account: ACCOUNT },
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

  it("renders an individual subtle row for each non-member", async () => {
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

    await screen.findByText("Alice Chen");
    expect(screen.getByText("u_ext_1")).toBeInTheDocument();
    expect(screen.getByText("u_ext_2")).toBeInTheDocument();
    expect(screen.queryByText(/Unidentified ·/)).not.toBeInTheDocument();

    // Unidentified rows sit after the named member, sorted by spend desc.
    const allRows = screen.getAllByRole("row");
    const aliceRow = allRows.find((r) => within(r).queryByText("Alice Chen"))!;
    const ext1Row = allRows.find((r) => within(r).queryByText("u_ext_1"))!;
    const ext2Row = allRows.find((r) => within(r).queryByText("u_ext_2"))!;
    expect(allRows.indexOf(aliceRow)).toBeLessThan(allRows.indexOf(ext1Row));
    expect(allRows.indexOf(ext1Row)).toBeLessThan(allRows.indexOf(ext2Row));
  });

  // Long real-row lists (named members + linked Slack users) collapse to 5
  // rows with a "Show top 10" control so the Insights page fits without an
  // outer scrollbar. Large lists move from top 5 → top 10 → all instead of
  // mounting every hidden row at once.
  it("collapses long real-row lists, then reveals top 10, then all rows", async () => {
    // Slack-format IDs land in the "real" bucket alongside named members.
    // SlackUserIdentity may render raw fallback labels or enriched profile
    // names, so the assertions match the stable raw ID substring.
    const users = Array.from({ length: 18 }, (_, i) =>
      makeUserRow({ user_id: `U07ABC${String(i).padStart(2, "0")}A`, cost_usd: 18 - i }),
    );
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    // Initial render: top 5 by spend visible, remaining 13 hidden.
    await screen.findByText(/U07ABC00A/);
    expect(screen.getByText(/U07ABC04A/)).toBeInTheDocument();
    expect(screen.queryByText(/U07ABC05A/)).not.toBeInTheDocument();
    expect(screen.queryByText(/U07ABC17A/)).not.toBeInTheDocument();

    const showMore = screen.getByRole("button", { name: /^Show top 10$/ });
    fireEvent.click(showMore);

    // First reveal: top 10 visible, remaining 8 still hidden, collapse remains available.
    expect(screen.getByText(/U07ABC05A/)).toBeInTheDocument();
    expect(screen.getByText(/U07ABC09A/)).toBeInTheDocument();
    expect(screen.queryByText(/U07ABC10A/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Show all$/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Show less$/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^Show all$/ }));
    expect(screen.getByText(/U07ABC17A/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Show all$/ })).not.toBeInTheDocument();
    const showLess = screen.getByRole("button", { name: /^Show less$/ });
    fireEvent.click(showLess);

    // Collapsed again — AnimatePresence keeps exiting rows in the DOM until
    // their exit animation completes, so wait for them to leave.
    await waitFor(() =>
      expect(screen.queryByText(/U07ABC05A/)).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("button", { name: /^Show top 10$/ })).toBeInTheDocument();
  });

  it("does not render the Show-more toggle when the real-row list fits in the default window", async () => {
    const users = Array.from({ length: 3 }, (_, i) =>
      makeUserRow({ user_id: `U07ABC0${i}A`, cost_usd: 3 - i }),
    );
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    await screen.findByText(/U07ABC00A/);
    expect(screen.queryByRole("button", { name: /Show \d+ more/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Show less/ })).not.toBeInTheDocument();
  });

  // User rows (real + unidentified) compete on the same sort — cost wins
  // regardless of identification. System spend is the only kind pinned last.
  it("ranks unidentified rows alongside real by cost; pins System spend last", async () => {
    const users = [
      makeUserRow({ user_id: "U07ABC01A", cost_usd: 100 }),
      makeUserRow({ user_id: "U07ABC02A", cost_usd: 80 }),
      makeUserRow({ user_id: "U07ABC03A", cost_usd: 60 }),
      // unidentified out-spends every real row → ranks #1 by cost desc.
      makeUserRow({ user_id: "anon-top", cost_usd: 500 }),
      makeUserRow({ user_id: "anon-low", cost_usd: 50 }),
      // System spend out-spends everything but stays pinned at the bottom.
      makeUserRow({ user_id: "", cost_usd: 999 }),
    ];
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    await screen.findByText(/U07ABC01A/);
    // Top 5 by cost: anon-top(500), U01(100), U02(80), U03(60), anon-low(50).
    // System spend (999) at #6, hidden behind Show-more.
    const visibleRows = screen
      .getAllByRole("row")
      .filter((r) => within(r).queryAllByRole("cell").length > 0);
    expect(visibleRows).toHaveLength(5);
    expect(within(visibleRows[0]).getByText("anon-top")).toBeInTheDocument();
    expect(within(visibleRows[1]).getByText(/U07ABC01A/)).toBeInTheDocument();
    expect(within(visibleRows[4]).getByText("anon-low")).toBeInTheDocument();
    expect(screen.queryByText("System spend")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Show all$/ })).toBeInTheDocument();
  });

  it("hides everything past row 5 behind Show-more when real overflows", async () => {
    const users = [
      // 6 real rows so real itself overflows the top-5 cut.
      ...Array.from({ length: 6 }, (_, i) =>
        makeUserRow({ user_id: `U07ABC0${i}A`, cost_usd: 100 - i }),
      ),
      // Unidentified + system tucked away until expansion.
      makeUserRow({ user_id: "weird-id-1", cost_usd: 9 }),
      makeUserRow({ user_id: "", cost_usd: 200 }),
    ];
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    await screen.findByText(/U07ABC00A/);
    // 6th real row hidden behind Show-more.
    expect(screen.queryByText(/U07ABC05A/)).not.toBeInTheDocument();
    // Unidentified + system also hidden — only revealed by Show-more.
    expect(screen.queryByText("weird-id-1")).not.toBeInTheDocument();
    expect(screen.queryByText("System spend")).not.toBeInTheDocument();
    // One click reveals all hidden rows because the full list fits within top 10.
    expect(screen.getByRole("button", { name: /^Show all$/ })).toBeInTheDocument();
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
          makeUserRow({ user_id: "U07ABCDEF", cost_usd: 4, user_details: { kind: "slack", team_id: "T07XYZ" } }),
        ]}
      />,
    );
    seedMembers(queryClient, []);

    const label = await screen.findByText("Slack user - U07ABCDEF");
    const anchor = label.closest("a");
    expect(anchor).not.toBeNull();
    expect(anchor!.getAttribute("href")).toBe("slack://user?team=T07XYZ&id=U07ABCDEF");
    expect(anchor!.querySelector(".rounded-full")).not.toBeNull();
  });

  it("renders enriched Slack profile metadata for unlinked users", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({
            user_id: "U07ABCDEF",
            cost_usd: 4,
            user_details: {
              kind: "slack",
              team_id: "T07XYZ",
              display_name: "Jesse Morgan",
              username: "jesse",
              avatar_url: "https://avatars.slack-edge.com/jesse.png",
            },
          }),
        ]}
      />,
    );
    seedMembers(queryClient, []);

    expect(await screen.findByText("jesse")).toBeInTheDocument();
    expect(screen.queryByText("Jesse Morgan")).not.toBeInTheDocument();
    expect(screen.queryByText("@jesse - U07ABCDEF")).not.toBeInTheDocument();
    expect(screen.queryByText("U07ABCDEF")).not.toBeInTheDocument();
    const avatar = document.querySelector('img[src="https://avatars.slack-edge.com/jesse.png"]');
    expect(avatar).not.toBeNull();
    expect(avatar!).toHaveAttribute("src", "https://avatars.slack-edge.com/jesse.png");
    expect(avatar!).toHaveClass("rounded-full");
    expect(avatar!).toHaveClass("opacity-60");
  });

  it("renders the same Slack user id as separate rows when workspaces differ", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({
            user_id: "U07SOHUM1",
            cost_usd: 7,
            user_details: {
              kind: "slack",
              team_id: "TPOSTMAN",
              display_name: "Sohum Dalal",
              username: "sohum",
            },
          }),
          makeUserRow({
            user_id: "U07SOHUM1",
            cost_usd: 5,
            user_details: {
              kind: "slack",
              team_id: "TASTRO",
              display_name: "Sohum Dalal",
              username: "sohum",
            },
          }),
        ]}
      />,
    );
    seedMembers(queryClient, []);

    expect(await screen.findAllByText("sohum")).toHaveLength(2);
    expect(screen.queryByText("(Postman) - @sohum - U07SOHUM1")).not.toBeInTheDocument();
    expect(screen.queryByText("(Astro) - @sohum - U07SOHUM1")).not.toBeInTheDocument();

    const slackLinks = screen.getAllByText("sohum").map((label) => label.closest("a")?.getAttribute("href"));
    expect(slackLinks).toEqual(expect.arrayContaining([
      "slack://user?team=TPOSTMAN&id=U07SOHUM1",
      "slack://user?team=TASTRO&id=U07SOHUM1",
    ]));
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
  // subtle Slack user row so a CEO scanning Insights can see which
  // workspace teammates are driving spend without first asking them to
  // link an Astro account.
  it("renders unlinked Slack users as per-id rows, not aggregated", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({ user_id: "U07ABCDEF", cost_usd: 4, user_details: { kind: "slack", team_id: "T07XYZ" } }),
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

  // A cross-account astro user (used a public blueprint from another org)
  // won't show up in the current account's member list. The row has
  // kind=astro + hydrated display_name + username from the server's
  // global personal-account lookup. The People table must render them
  // with their real name + handle, not fall into the Unidentified bucket
  // or get rendered as a Slack user.
  it("renders cross-account astro users via hydrated user_details when not in the member list", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          // Bob doesn't belong to ACCOUNT, but the server hydrated his
          // personal-account info so the row can still render properly.
          makeUserRow({
            user_id: "user_01HXX_bob",
            cost_usd: 4,
            user_details: { kind: "astro", display_name: "Bob Smith", username: "bob" },
          }),
        ]}
      />,
    );
    // Empty member list — Bob isn't a member of ACCOUNT.
    seedMembers(queryClient, []);

    // Display name renders (not the raw user_01HXX_bob, not a Slack badge).
    expect(await screen.findByText("Bob Smith")).toBeInTheDocument();
    expect(screen.queryByText(/Slack user - /)).not.toBeInTheDocument();
    expect(screen.queryByText(/Unidentified · /)).not.toBeInTheDocument();
    // The profile link target uses the hydrated username slug.
    const link = screen.getByRole("link", { name: /Bob Smith/ });
    expect(link.getAttribute("href")).toBe("/bob");
  });

  // Cross-account astro WITHOUT hydration (server lookup missed — e.g.
  // user deleted between trace emission and read) falls into the
  // unidentified bucket rather than crashing or rendering a broken link.
  it("falls back to unidentified for astro users with no hydrated user_details", async () => {
    const { queryClient } = renderWithProviders(
      <TopSpendersTable
        mode="users"
        account={ACCOUNT}
        loading={false}
        users={[
          makeUserRow({
            user_id: "user_01HXX_ghost",
            cost_usd: 4,
            user_details: { kind: "astro" }, // no display_name / username
          }),
        ]}
      />,
    );
    seedMembers(queryClient, []);

    // No profile link — the row is unidentified.
    await screen.findByText(/user_01HXX_ghost|Unidentified/);
    expect(screen.queryByRole("link", { name: /user_01HXX_ghost/ })).not.toBeInTheDocument();
  });

  // Rank prefix reflects position in the visible list. It lives inside the
  // Name cell before the identity label, so the table keeps one coherent
  // identity column instead of exposing a separate Rank header.
  function rankOf(row: HTMLElement): string {
    return within(row).getAllByRole("cell")[0]?.textContent?.trim().match(/^\d+/)?.[0] ?? "";
  }
  function userRows(): HTMLElement[] {
    return screen.getAllByRole("row").filter((r) => /U07/.test(r.textContent ?? ""));
  }

  it("numbers the top 5 rows 1..5 in display order", async () => {
    const users = Array.from({ length: 7 }, (_, i) =>
      makeUserRow({ user_id: `U07RANK0${i}A`, cost_usd: 100 - i }),
    );
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    await screen.findByText(/U07RANK00A/);
    const rows = userRows();
    expect(rows).toHaveLength(5);
    expect(rows.map(rankOf)).toEqual(["1", "2", "3", "4", "5"]);
  });

  it("continues rank numbering past 5 after Show-more expands", async () => {
    const users = Array.from({ length: 7 }, (_, i) =>
      makeUserRow({ user_id: `U07RANK0${i}A`, cost_usd: 100 - i }),
    );
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    await screen.findByText(/U07RANK00A/);
    fireEvent.click(screen.getByRole("button", { name: /^Show all$/ }));

    const rows = userRows();
    expect(rows).toHaveLength(7);
    expect(rows.map(rankOf)).toEqual(["1", "2", "3", "4", "5", "6", "7"]);
  });

  it("re-ranks rows when the sort key changes", async () => {
    // Cost order and requests order differ so the rank re-shuffles on toggle.
    const users = [
      makeUserRow({ user_id: "U07RANKAA", cost_usd: 100, requests: 1 }),
      makeUserRow({ user_id: "U07RANKBA", cost_usd: 50, requests: 99 }),
      makeUserRow({ user_id: "U07RANKCA", cost_usd: 75, requests: 50 }),
    ];
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    await screen.findByText(/U07RANKAA/);
    // Default sort = cost desc → AA(100) #1, CA(75) #2, BA(50) #3.
    const beforeRows = userRows();
    expect(within(beforeRows[0]).getByText(/U07RANKAA/)).toBeInTheDocument();
    expect(within(beforeRows[2]).getByText(/U07RANKBA/)).toBeInTheDocument();
    expect(beforeRows.map(rankOf)).toEqual(["1", "2", "3"]);

    // Switch sort to Requests → desc → BA(99) #1, CA(50) #2, AA(1) #3.
    fireEvent.click(screen.getByText("Requests"));
    const afterRows = userRows();
    expect(within(afterRows[0]).getByText(/U07RANKBA/)).toBeInTheDocument();
    expect(within(afterRows[2]).getByText(/U07RANKAA/)).toBeInTheDocument();
    expect(afterRows.map(rankOf)).toEqual(["1", "2", "3"]);
  });

  it("re-ranks rows when sort direction toggles", async () => {
    const users = [
      makeUserRow({ user_id: "U07RANKAA", cost_usd: 100 }),
      makeUserRow({ user_id: "U07RANKBA", cost_usd: 50 }),
      makeUserRow({ user_id: "U07RANKCA", cost_usd: 25 }),
    ];
    const { queryClient } = renderWithProviders(
      <TopSpendersTable mode="users" account={ACCOUNT} loading={false} users={users} />,
    );
    seedMembers(queryClient, []);

    await screen.findByText(/U07RANKAA/);
    // Default desc → AA #1, BA #2, CA #3.
    const descRows = userRows();
    expect(within(descRows[0]).getByText(/U07RANKAA/)).toBeInTheDocument();
    expect(within(descRows[2]).getByText(/U07RANKCA/)).toBeInTheDocument();

    // Click Spend header → toggles to asc → CA #1, BA #2, AA #3.
    fireEvent.click(screen.getByText("Spend"));
    const ascRows = userRows();
    expect(within(ascRows[0]).getByText(/U07RANKCA/)).toBeInTheDocument();
    expect(within(ascRows[2]).getByText(/U07RANKAA/)).toBeInTheDocument();
    expect(ascRows.map(rankOf)).toEqual(["1", "2", "3"]);
  });
});
