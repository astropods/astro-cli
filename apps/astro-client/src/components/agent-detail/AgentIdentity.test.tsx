import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import type { AgentDeployment } from "@/lib/api";
import { AgentIdentity } from "./AgentIdentity";

// Stub the query hooks the identity row + switcher read.
vi.mock("@/api/queries/deployments", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/queries/deployments")>();
  return {
    ...actual,
    useDeploymentsSummary: (() => ({
      data: {
        accounts: [
          {
            id: "acct-1",
            name: "acme",
            display_name: "Acme",
            deployments: [
              { id: "dep-1", name: "test-agent", display_name: "Test Agent" },
            ],
          },
        ],
      },
    })) as unknown as typeof actual.useDeploymentsSummary,
    useRestartDeployment: (() => ({
      mutate: vi.fn(),
    })) as unknown as typeof actual.useRestartDeployment,
  };
});

vi.mock("@/api/queries/blueprints", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/queries/blueprints")>();
  return {
    ...actual,
    useBlueprint: (() => ({ data: undefined })) as unknown as typeof actual.useBlueprint,
  };
});

// Heavy modal deps, not under test here.
vi.mock("@/components/trading-card/TradingCardModal", () => ({
  TradingCardModal: () => null,
}));
vi.mock("@/components/DeleteDeploymentDialog", () => ({
  DeleteDeploymentDialog: () => null,
}));

const deployment: AgentDeployment = {
  id: "dep-1",
  name: "test-agent",
  display_name: "Test Agent",
  build_id: "b1",
  namespace: "ns",
  status: "Running",
  replicas: 1,
  created_at: "2026-01-01T00:00:00Z",
  components: [],
};

const originalMatchMedia = window.matchMedia;

// matches === mobile (useMediaBreakpoint reads `(max-width: 1023px)`).
function setViewport(isMobile: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: isMobile,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

function renderIdentity() {
  return render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <AgentIdentity account="acme" deployment={deployment} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  window.matchMedia = originalMatchMedia;
});

describe("AgentIdentity actions menu", () => {
  it("allows the selector to shrink inside the shared top bar", () => {
    setViewport(false);
    renderIdentity();

    const trigger = screen.getByRole("button", { name: /agent menu/i });
    expect(trigger).toHaveClass("min-w-0", "max-w-full");
    expect(trigger.parentElement).toHaveClass("min-w-0", "max-w-full");
  });

  it("shows a standalone actions kebab with all actions on desktop", async () => {
    setViewport(false);
    renderIdentity();

    await userEvent.setup().click(
      screen.getByRole("button", { name: /agent actions/i }),
    );

    expect(screen.getByRole("menuitem", { name: /view blueprint/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /share agent badge/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /restart deployment/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /delete agent/i })).toBeInTheDocument();
  });

  it("keeps the actions out of the agent selector menu on desktop", async () => {
    setViewport(false);
    renderIdentity();

    await userEvent.setup().click(
      screen.getByRole("button", { name: /agent menu/i }),
    );

    expect(screen.getByRole("menu")).toBeInTheDocument();
    expect(
      screen.queryByRole("menuitem", { name: /restart deployment/i }),
    ).not.toBeInTheDocument();
  });

  it("renders the current agent as a non-navigable selected row", async () => {
    setViewport(false);
    renderIdentity();

    await userEvent.setup().click(
      screen.getByRole("button", { name: /agent menu/i }),
    );

    const menu = screen.getByRole("menu");
    expect(within(menu).getByText("Test Agent").closest("[aria-current]")).not.toBeNull();
    expect(
      within(menu).queryByRole("menuitem", { name: /test agent/i }),
    ).not.toBeInTheDocument();
    expect(
      within(menu).queryByRole("link", { name: /test agent/i }),
    ).not.toBeInTheDocument();
  });

  it("folds the actions into the selector menu on mobile (no kebab)", async () => {
    setViewport(true);
    renderIdentity();

    expect(
      screen.queryByRole("button", { name: /agent actions/i }),
    ).not.toBeInTheDocument();

    await userEvent.setup().click(
      screen.getByRole("button", { name: /agent menu/i }),
    );

    expect(
      screen.getByRole("menuitem", { name: /restart deployment/i }),
    ).toBeInTheDocument();
  });
});
