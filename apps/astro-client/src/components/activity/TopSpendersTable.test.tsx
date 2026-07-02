import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "@/test/test-utils";
import type { InsightsAgentRow, InsightsPersonRow } from "@/lib/api";
import { TopSpendersTable } from "./TopSpendersTable";

afterEach(cleanup);

function agentRow(overrides: Partial<InsightsAgentRow> & { key: string; label: string }): InsightsAgentRow {
  const { key, label, ...rowOverrides } = overrides;
  return {
    key,
    search_text: label.toLowerCase(),
    identity: {
      kind: "agent",
      id: key,
      label,
      href: `/acme/agents/${key}/monitor`,
      avatar_account: "acme",
      avatar_name: label.toLowerCase().replace(/\s+/g, "-"),
    },
    used_by: [],
    metrics: {
      requests: 5,
      cost_usd: 10,
      cost_pct: 10,
      cost_per_request: 2,
      tok_per_request: 100,
      p95_latency_ms: 300,
    },
    ...rowOverrides,
  };
}

function personRow(overrides: Partial<InsightsPersonRow> & { key: string; label: string }): InsightsPersonRow {
  const { key, label, ...rowOverrides } = overrides;
  return {
    key,
    search_text: label.toLowerCase(),
    identity: {
      kind: "member",
      id: key,
      label,
      href: `/${label.toLowerCase().replace(/\s+/g, "-")}`,
      avatar_handle: label.toLowerCase().replace(/\s+/g, "-"),
    },
    agents_used: [],
    metrics: {
      requests: 1,
      cost_usd: 1,
      cost_pct: 10,
      tokens: 100,
      last_seen: "2026-06-01T00:00:00Z",
    },
    ...rowOverrides,
  };
}

function bodyRows() {
  return screen
    .getAllByRole("row")
    .filter((row) => within(row).queryAllByRole("cell").length > 0);
}

function firstCellText(row: HTMLElement): string {
  return within(row).getAllByRole("cell")[0]?.textContent?.trim() ?? "";
}

describe("TopSpendersTable agents mode", () => {
  it("shows ghost rows when loading", () => {
    const { container } = renderWithProviders(
      <TopSpendersTable mode="agents" rows={[]} loading={true} />,
    );
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  it("shows an empty state when there are no rows", () => {
    renderWithProviders(<TopSpendersTable mode="agents" rows={[]} loading={false} />);
    expect(screen.getByText("No deployment activity in this period")).toBeInTheDocument();
  });

  it("renders server-shaped agent rows and used-by identities", async () => {
    const user = userEvent.setup();

    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={[
          agentRow({
            key: "dep-alpha",
            label: "Alpha Agent",
            used_by: [
              {
                kind: "member",
                id: "u_alice",
                label: "Alice Chen",
                href: "/alice",
                avatar_handle: "alice",
              },
              {
                kind: "slack",
                id: "U07ABCDEF",
                label: "Christopher Patty",
                href: "slack://user?team=T07XYZ&id=U07ABCDEF",
                tooltip: "Slack User",
              },
            ],
          }),
        ]}
      />,
    );

    const row = bodyRows()[0];
    expect(within(row).getByText("Alpha Agent")).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: /Alpha Agent/ })).toHaveAttribute(
      "href",
      "/acme/agents/dep-alpha/monitor",
    );
    expect(within(row).getAllByRole("img")).toHaveLength(2);

    await user.hover(screen.getByLabelText("Christopher Patty"));
    expect((await screen.findAllByText("Christopher Patty")).length).toBeGreaterThan(0);
    expect(screen.queryByText("Slack User")).not.toBeInTheDocument();
  });

  it("renders rank inside the agent identity cell", () => {
    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={[
          agentRow({ key: "dep-alpha", label: "Alpha Agent" }),
          agentRow({ key: "dep-beta", label: "Beta Agent", metrics: { requests: 1, cost_usd: 5, cost_pct: 5, cost_per_request: 1, tok_per_request: 100, p95_latency_ms: 100 } }),
        ]}
      />,
    );

    expect(screen.queryByRole("columnheader", { name: "Rank" })).not.toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Name" })).toBeInTheDocument();
    expect(firstCellText(bodyRows()[0])).toMatch(/^1Alpha Agent/);
    expect(firstCellText(bodyRows()[1])).toMatch(/^2Beta Agent/);
  });

  it("renders a not-instrumented marker for uninstrumented agent rows", async () => {
    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={[
          agentRow({
            key: "dep-alpha",
            label: "Alpha Agent",
            not_instrumented: true,
          }),
        ]}
      />,
    );

    const marker = screen.getByLabelText("Not instrumented");
    expect(marker).toBeInTheDocument();
    fireEvent.focus(marker);
    expect(await screen.findAllByText(/Instrumentation not available/)).not.toHaveLength(0);
  });

  it("renders used-by overflow behind a +N popover", async () => {
    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={[
          agentRow({
            key: "dep-alpha",
            label: "Alpha Agent",
            used_by: Array.from({ length: 5 }, (_, index) => ({
              kind: "member",
              id: `u_${index}`,
              label: `User ${index}`,
              href: `/user-${index}`,
              avatar_handle: `user-${index}`,
            })),
          }),
        ]}
      />,
    );

    const overflow = screen.getByRole("button", { name: "Show 5 people" });
    expect(overflow).toHaveTextContent("+2");
    fireEvent.click(overflow);
    expect(await screen.findByText("5 people")).toBeInTheDocument();
    expect(screen.getByText("User 4")).toBeInTheDocument();
  });

  it("sorts agent rows locally by the active column", () => {
    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={[
          agentRow({ key: "dep-low", label: "Low", metrics: { requests: 1, cost_usd: 1, cost_pct: 10, cost_per_request: 1, tok_per_request: 100, p95_latency_ms: 100 } }),
          agentRow({ key: "dep-high", label: "High", metrics: { requests: 10, cost_usd: 10, cost_pct: 90, cost_per_request: 1, tok_per_request: 100, p95_latency_ms: 100 } }),
        ]}
      />,
    );

    expect(within(bodyRows()[0]).getByText("High")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("columnheader", { name: "Spend" }));
    expect(within(bodyRows()[0]).getByText("Low")).toBeInTheDocument();
  });

  it("collapses long lists behind Show more", () => {
    const rows = Array.from({ length: 7 }, (_, index) =>
      agentRow({
        key: `dep-${index}`,
        label: `Agent ${index}`,
        metrics: {
          requests: 1,
          cost_usd: 100 - index,
          cost_pct: 10,
          cost_per_request: 1,
          tok_per_request: 100,
          p95_latency_ms: 100,
        },
      }),
    );
    const { rerender } = renderWithProviders(<TopSpendersTable mode="agents" loading={false} rows={rows} />);

    expect(screen.getByText("Agent 0")).toBeInTheDocument();
    expect(screen.queryByText("Agent 6")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show 2 more" }));
    expect(screen.getByText("Agent 6")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show less" }));
    expect(screen.queryByText("Agent 6")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show 2 more" }));
    expect(screen.getByText("Agent 6")).toBeInTheDocument();
    rerender(<TopSpendersTable mode="agents" loading={false} rows={rows.slice(0, 6)} />);
    expect(screen.queryByText("Agent 5")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show 1 more" })).toBeInTheDocument();
  });

  it("uses server pagination and controlled sort callbacks when provided", () => {
    const onShowMore = vi.fn();
    const onShowLess = vi.fn();
    const onSort = vi.fn();

    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={Array.from({ length: 5 }, (_, index) =>
          agentRow({
            key: `dep-${index}`,
            label: `Agent ${index}`,
            metrics: {
              requests: index + 1,
              cost_usd: index + 1,
              cost_pct: 10,
              cost_per_request: 1,
              tok_per_request: 100,
              p95_latency_ms: 100,
            },
          }),
        )}
        sortKey="requests"
        sortDirection="desc"
        onSort={onSort}
        pagination={{ totalRows: 12, onShowMore, onShowLess }}
      />,
    );

    fireEvent.click(screen.getByRole("columnheader", { name: "Requests" }));
    expect(onSort).toHaveBeenCalledWith("requests");
    fireEvent.click(screen.getByRole("button", { name: "Show 7 more" }));
    expect(onShowMore).toHaveBeenCalledTimes(1);
  });

  it("shows Show top 25 when expanded past the Insights default window", () => {
    const onShowMore = vi.fn();
    const onShowLess = vi.fn();
    const pagination = {
      totalRows: 50,
      defaultVisibleRows: 25,
      pageSize: 10,
      showLessLabel: "Show top 25",
      onShowMore,
      onShowLess,
    };

    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={Array.from({ length: 35 }, (_, index) =>
          agentRow({
            key: `dep-${index}`,
            label: `Agent ${index}`,
            metrics: {
              requests: index + 1,
              cost_usd: index + 1,
              cost_pct: 10,
              cost_per_request: 1,
              tok_per_request: 100,
              p95_latency_ms: 100,
            },
          }),
        )}
        pagination={pagination}
      />,
    );

    expect(screen.getByRole("button", { name: "Show 10 more" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show top 25" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show top 25" }));
    expect(onShowLess).toHaveBeenCalledTimes(1);
  });

  it("does not show collapse affordance at the default Insights window", () => {
    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={Array.from({ length: 25 }, (_, index) =>
          agentRow({
            key: `dep-${index}`,
            label: `Agent ${index}`,
            metrics: {
              requests: index + 1,
              cost_usd: index + 1,
              cost_pct: 10,
              cost_per_request: 1,
              tok_per_request: 100,
              p95_latency_ms: 100,
            },
          }),
        )}
        pagination={{
          totalRows: 50,
          defaultVisibleRows: 25,
          pageSize: 10,
          showLessLabel: "Show top 25",
          onShowMore: vi.fn(),
          onShowLess: vi.fn(),
        }}
      />,
    );

    expect(screen.queryByRole("button", { name: "Show top 25" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show 10 more" })).toBeInTheDocument();
  });

  it("keeps collapse affordance when all rows are loaded past the default window", () => {
    renderWithProviders(
      <TopSpendersTable
        mode="agents"
        loading={false}
        rows={Array.from({ length: 40 }, (_, index) =>
          agentRow({
            key: `dep-${index}`,
            label: `Agent ${index}`,
            metrics: {
              requests: index + 1,
              cost_usd: index + 1,
              cost_pct: 10,
              cost_per_request: 1,
              tok_per_request: 100,
              p95_latency_ms: 100,
            },
          }),
        )}
        pagination={{
          totalRows: 40,
          defaultVisibleRows: 25,
          pageSize: 10,
          showLessLabel: "Show top 25",
          onShowMore: vi.fn(),
          onShowLess: vi.fn(),
        }}
      />,
    );

    expect(screen.getByRole("button", { name: "Show top 25" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Show \d+ more/ })).not.toBeInTheDocument();
  });
});

describe("TopSpendersTable users mode", () => {
  it("shows ghost rows when loading", () => {
    const { container } = renderWithProviders(
      <TopSpendersTable mode="users" rows={[]} loading={true} />,
    );
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  it("shows an empty state when there are no rows", () => {
    renderWithProviders(<TopSpendersTable mode="users" rows={[]} loading={false} />);
    expect(screen.getByText("No activity from people in this period")).toBeInTheDocument();
  });

  it("renders member, Slack, and system identities from the server row model", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <TopSpendersTable
        mode="users"
        loading={false}
        rows={[
          personRow({
            key: "member:u_alice",
            label: "Alice Chen",
            agents_used: [
              {
                key: "dep-alpha",
                label: "Alpha Agent",
                href: "/acme/agents/dep-alpha/monitor",
                avatar_account: "acme",
                avatar_name: "alpha",
              },
            ],
            metrics: { requests: 3, cost_usd: 3, cost_pct: 30, tokens: 300, last_seen: "2026-06-01T00:00:00Z" },
          }),
          personRow({
            key: "slack:T07XYZ:U07ABCDEF",
            label: "Christopher Patty",
            identity: {
              kind: "slack",
              id: "U07ABCDEF",
              label: "Christopher Patty",
              href: "slack://user?team=T07XYZ&id=U07ABCDEF",
              tooltip: "Slack User",
            },
            metrics: { requests: 2, cost_usd: 2, cost_pct: 20, tokens: 200, last_seen: "2026-06-01T00:00:00Z" },
          }),
          personRow({
            key: "system:__system_spend__",
            label: "System spend",
            identity: {
              kind: "system",
              id: "__system_spend__",
              label: "System spend",
              tooltip: "Traces not associated with any user.",
            },
            metrics: { requests: 1, cost_usd: 1, cost_pct: 10, tokens: 100 },
          }),
        ]}
      />,
    );

    expect(screen.getByText("Alice Chen")).toBeInTheDocument();
    expect(screen.getByText("Alpha Agent")).toBeInTheDocument();
    const slackLink = screen.getByRole("link", { name: /Christopher Patty/ });
    expect(slackLink).toHaveAttribute(
      "href",
      "slack://user?team=T07XYZ&id=U07ABCDEF",
    );
    await user.hover(slackLink);
    expect((await screen.findAllByText("Slack User")).length).toBeGreaterThan(0);
    expect(screen.getByText("System spend")).toBeInTheDocument();
  });

  it("renders rank inside the people identity cell", () => {
    renderWithProviders(
      <TopSpendersTable
        mode="users"
        loading={false}
        rows={[
          personRow({ key: "member:alice", label: "Alice Chen", metrics: { requests: 1, cost_usd: 10, cost_pct: 10, tokens: 100 } }),
          personRow({ key: "member:bob", label: "Bob Smith", metrics: { requests: 1, cost_usd: 5, cost_pct: 5, tokens: 100 } }),
        ]}
      />,
    );

    expect(screen.queryByRole("columnheader", { name: "Rank" })).not.toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Name" })).toBeInTheDocument();
    expect(firstCellText(bodyRows()[0])).toMatch(/^1Alice Chen/);
    expect(firstCellText(bodyRows()[1])).toMatch(/^2Bob Smith/);
  });

  it("renders unidentified users as mono text without a link", () => {
    renderWithProviders(
      <TopSpendersTable
        mode="users"
        loading={false}
        rows={[
          personRow({
            key: "unidentified:random-trace-id",
            label: "random-trace-id",
            identity: {
              kind: "unidentified",
              id: "random-trace-id",
              label: "random-trace-id",
            },
          }),
        ]}
      />,
    );

    const label = screen.getByText("random-trace-id");
    expect(label.closest(".font-mono")).not.toBeNull();
    expect(screen.queryByRole("link", { name: /random-trace-id/ })).not.toBeInTheDocument();
  });

  it("renders agents-used overflow behind a +N popover", async () => {
    renderWithProviders(
      <TopSpendersTable
        mode="users"
        loading={false}
        rows={[
          personRow({
            key: "member:u_alice",
            label: "Alice Chen",
            agents_used: Array.from({ length: 5 }, (_, index) => ({
              key: `dep-${index}`,
              label: `Agent ${index}`,
              href: `/acme/agents/dep-${index}/monitor`,
              avatar_account: "acme",
              avatar_name: `agent-${index}`,
            })),
          }),
        ]}
      />,
    );

    const overflow = screen.getByRole("button", { name: "Show 5 agents" });
    expect(overflow).toHaveTextContent("+2");
    fireEvent.click(overflow);
    expect(await screen.findByText("5 agents")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Agent 4/ })).toHaveAttribute(
      "href",
      "/acme/agents/dep-4/monitor",
    );
  });

  it("keeps system spend pinned last while user rows sort by spend", () => {
    renderWithProviders(
      <TopSpendersTable
        mode="users"
        loading={false}
        rows={[
          personRow({ key: "member:low", label: "Low Spender", metrics: { requests: 1, cost_usd: 1, cost_pct: 1, tokens: 100 } }),
          personRow({ key: "member:high", label: "High Spender", metrics: { requests: 1, cost_usd: 10, cost_pct: 10, tokens: 100 } }),
          personRow({
            key: "system:__system_spend__",
            label: "System spend",
            identity: { kind: "system", id: "__system_spend__", label: "System spend" },
            metrics: { requests: 1, cost_usd: 999, cost_pct: 90, tokens: 100 },
          }),
        ]}
      />,
    );

    const rows = bodyRows();
    expect(within(rows[0]).getByText("High Spender")).toBeInTheDocument();
    expect(within(rows[1]).getByText("Low Spender")).toBeInTheDocument();
    expect(within(rows[2]).getByText("System spend")).toBeInTheDocument();
  });
});
