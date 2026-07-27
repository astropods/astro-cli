import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { AgentDeploymentMenu } from "./AgentDeploymentMenu";

// The switcher lists whatever useDeploymentsSummary returns; each test sets the
// accounts (in server order) to check how the menu reorders them.
const state = vi.hoisted(() => ({ accounts: [] as unknown[] }));

vi.mock("@/api/queries/deployments", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/queries/deployments")>();
  return {
    ...actual,
    useDeploymentsSummary: (() => ({
      data: { accounts: state.accounts },
    })) as unknown as typeof actual.useDeploymentsSummary,
  };
});

function acct(
  type: string,
  name: string,
  deployments: { id: string; name: string; display_name: string }[],
) {
  return { id: `acct-${name}`, name, type, display_name: name, deployments };
}

const current = { id: "dep-org", name: "org-agent", display_name: "Org Agent" };

function renderMenu({ deployMoreHref }: { deployMoreHref?: string } = {}) {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <AgentDeploymentMenu
          deployment={current}
          getDeploymentPath={(account, dep) => `/${account}/agents/${dep.id}`}
          showAccountLabels
          deployMoreHref={deployMoreHref}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function labelOrder(): string[] {
  const listbox = screen.getByRole("listbox", { name: "Agents" });
  return within(listbox)
    .getAllByText(/^(Personal|Acme|Beta)$/)
    .map((el) => el.textContent ?? "");
}

afterEach(() => {
  cleanup();
  state.accounts = [];
});

describe("AgentDeploymentMenu account ordering", () => {
  it("lists the current account before the personal account", async () => {
    state.accounts = [
      acct("organization", "Acme", [current]),
      acct("personal", "Personal", [
        { id: "dep-me", name: "my-agent", display_name: "My Agent" },
      ]),
    ];

    renderMenu();
    await userEvent.setup().click(screen.getByRole("button", { name: /agent menu/i }));

    expect(labelOrder()).toEqual(["Acme", "Personal"]);
  });

  it("keeps personal-first ordering for the remaining accounts", async () => {
    state.accounts = [
      acct("organization", "Beta", [
        { id: "dep-beta", name: "beta-agent", display_name: "Beta Agent" },
      ]),
      acct("personal", "Personal", [
        { id: "dep-me", name: "my-agent", display_name: "My Agent" },
      ]),
      acct("organization", "Acme", [current]),
    ];

    renderMenu();
    await userEvent.setup().click(screen.getByRole("button", { name: /agent menu/i }));

    expect(labelOrder()).toEqual(["Acme", "Personal", "Beta"]);
  });

  it("lists the current agent first within its account", async () => {
    state.accounts = [
      acct("organization", "Acme", [
        { id: "dep-one", name: "one", display_name: "Agent One" },
        current,
        { id: "dep-two", name: "two", display_name: "Agent Two" },
      ]),
    ];

    renderMenu();
    await userEvent.setup().click(screen.getByRole("button", { name: /agent menu/i }));

    const listbox = screen.getByRole("listbox", { name: "Agents" });
    const currentAgent = within(listbox).getByText("Org Agent");
    const nextAgent = within(listbox).getByText("Agent One");
    expect(
      currentAgent.compareDocumentPosition(nextAgent) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});

describe("AgentDeploymentMenu search", () => {
  it("uses non-modal searchbox and listbox semantics", async () => {
    state.accounts = [acct("organization", "Acme", [current])];

    renderMenu();
    const trigger = screen.getByRole("button", { name: /agent menu/i });
    await userEvent.setup().click(trigger);

    const listbox = screen.getByRole("listbox", { name: "Agents" });
    const search = screen.getByRole("searchbox", { name: "Search agents" });
    const panel = search.closest('[data-slot="popover-content"]');

    expect(panel?.firstElementChild).toContainElement(search);
    await waitFor(() => expect(search).toHaveFocus());
    expect(search).toHaveAttribute("aria-controls", listbox.id);
    expect(trigger).toHaveAttribute("aria-haspopup", "listbox");
    expect(trigger).toHaveAttribute("aria-controls", listbox.id);
    expect(panel).not.toHaveAttribute("role");
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(panel).toHaveClass(
      "border-border",
      "bg-popover",
      "text-popover-foreground",
    );
    expect(panel).not.toHaveClass("bg-stone-800", "text-xs");
    expect(
      panel?.querySelector('[data-slot="scroll-area"]'),
    ).toBeInTheDocument();
  });

  it("does not cap the agents shown for an account", async () => {
    state.accounts = [
      acct("organization", "Acme", [
        current,
        { id: "dep-one", name: "one", display_name: "Agent One" },
        { id: "dep-two", name: "two", display_name: "Agent Two" },
        { id: "dep-three", name: "three", display_name: "Agent Three" },
        { id: "dep-four", name: "four", display_name: "Agent Four" },
      ]),
    ];

    renderMenu();
    await userEvent.setup().click(screen.getByRole("button", { name: /agent menu/i }));

    const listbox = screen.getByRole("listbox", { name: "Agents" });
    expect(within(listbox).getAllByRole("option")).toHaveLength(5);
    expect(within(listbox).getByText("Agent Four")).toBeInTheDocument();
  });

  it("searches agent names across every account and hides empty groups", async () => {
    state.accounts = [
      acct("personal", "Personal", [
        { id: "dep-notes", name: "meeting-notes", display_name: "Meeting Notes" },
      ]),
      acct("organization", "Acme", [
        current,
        { id: "dep-design", name: "design-review", display_name: "Design Review" },
      ]),
      acct("organization", "Beta", [
        { id: "dep-billing", name: "billing-assistant", display_name: "Billing Assistant" },
      ]),
    ];

    renderMenu();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /agent menu/i }));
    await user.type(screen.getByRole("searchbox", { name: "Search agents" }), "billing");

    expect(screen.getByText("Billing Assistant")).toBeInTheDocument();
    expect(screen.getByText("Beta")).toBeInTheDocument();
    expect(screen.queryByText("Meeting Notes")).not.toBeInTheDocument();
    expect(screen.queryByText("Design Review")).not.toBeInTheDocument();
    expect(screen.queryByText("Acme")).not.toBeInTheDocument();
  });

  it("skips the clear button when ArrowDown enters filtered results", async () => {
    state.accounts = [
      acct("organization", "Acme", [
        current,
        { id: "dep-one", name: "one", display_name: "Agent One" },
        { id: "dep-two", name: "two", display_name: "Agent Two" },
      ]),
    ];

    renderMenu();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /agent menu/i }));

    const search = screen.getByRole("searchbox", { name: "Search agents" });
    await user.type(search, "one");
    const clear = screen.getByRole("button", { name: "Clear agent search" });

    await user.tab();
    expect(clear).toHaveFocus();
    await user.tab({ shift: true });
    expect(search).toHaveFocus();

    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: "Agent One" })).toHaveFocus();
  });

  it("matches account names and keeps all agents in the matching account", async () => {
    state.accounts = [
      acct("organization", "Acme", [
        current,
        { id: "dep-design", name: "design-review", display_name: "Design Review" },
      ]),
      acct("organization", "Beta", [
        { id: "dep-billing", name: "billing-assistant", display_name: "Billing Assistant" },
      ]),
    ];

    renderMenu();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /agent menu/i }));
    await user.type(screen.getByRole("searchbox", { name: "Search agents" }), "acme");

    const listbox = screen.getByRole("listbox", { name: "Agents" });
    expect(within(listbox).getByText("Org Agent")).toBeInTheDocument();
    expect(within(listbox).getByText("Design Review")).toBeInTheDocument();
    expect(within(listbox).queryByText("Billing Assistant")).not.toBeInTheDocument();
  });

  it("shows a directional empty state and restores results when cleared", async () => {
    state.accounts = [acct("organization", "Acme", [current])];

    renderMenu();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /agent menu/i }));
    await user.type(screen.getByRole("searchbox", { name: "Search agents" }), "missing");

    expect(screen.getByText("No agents found")).toBeInTheDocument();
    expect(screen.getByText("Try another agent or account name.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Clear agent search" }));
    expect(
      within(screen.getByRole("listbox", { name: "Agents" })).getByText(
        "Org Agent",
      ),
    ).toBeInTheDocument();
  });

  it("moves from search into results and lets Escape and boundary Tab leave", async () => {
    state.accounts = [
      acct("organization", "Acme", [
        current,
        { id: "dep-one", name: "one", display_name: "Agent One" },
      ]),
    ];

    renderMenu();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /agent menu/i }));

    const search = screen.getByRole("searchbox", { name: "Search agents" });
    await waitFor(() => expect(search).toHaveFocus());
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: "Agent One" })).toHaveFocus();

    await user.keyboard("{ArrowUp}");
    expect(search).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox", { name: "Agents" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /agent menu/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("searchbox", { name: "Search agents" }),
      ).toHaveFocus(),
    );
    await user.tab();
    expect(screen.queryByRole("listbox", { name: "Agents" })).not.toBeInTheDocument();
  });

  it("reaches the deploy-more footer with Tab and ArrowDown", async () => {
    state.accounts = [acct("organization", "Acme", [current])];

    renderMenu({ deployMoreHref: "/agents/new" });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /agent menu/i }));

    const search = screen.getByRole("searchbox", { name: "Search agents" });
    const deployMore = screen.getByRole("link", { name: "Deploy more agents" });
    await waitFor(() => expect(search).toHaveFocus());
    await user.tab();
    expect(deployMore).toHaveFocus();

    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: /agent menu/i }));
    await waitFor(() =>
      expect(
        screen.getByRole("searchbox", { name: "Search agents" }),
      ).toHaveFocus(),
    );
    await user.keyboard("{ArrowDown}");
    expect(
      screen.getByRole("link", { name: "Deploy more agents" }),
    ).toHaveFocus();
  });
});
