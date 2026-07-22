import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, within } from "@testing-library/react";
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

function renderMenu() {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <AgentDeploymentMenu
          deployment={current}
          getDeploymentPath={(account, dep) => `/${account}/agents/${dep.id}`}
          showAccountLabels
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function labelOrder(): string[] {
  const menu = screen.getByRole("menu");
  return within(menu)
    .getAllByText(/^(Personal|Acme|Beta)$/)
    .map((el) => el.textContent ?? "");
}

afterEach(cleanup);

describe("AgentDeploymentMenu account ordering", () => {
  it("lists the personal account group before organization groups", async () => {
    state.accounts = [
      acct("organization", "Acme", [current]),
      acct("personal", "Personal", [
        { id: "dep-me", name: "my-agent", display_name: "My Agent" },
      ]),
    ];

    renderMenu();
    await userEvent.setup().click(screen.getByRole("button", { name: /agent menu/i }));

    expect(labelOrder()).toEqual(["Personal", "Acme"]);
  });

  it("alphabetizes organization accounts after the personal group", async () => {
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

    // Personal first, then Acme before Beta (server order was Beta, Acme).
    expect(labelOrder()).toEqual(["Personal", "Acme", "Beta"]);
  });
});
