import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import type { AgentDeploymentSummary } from "@/lib/api";
import type { ChatSession } from "@/lib/chat/types";
import { ChatThreadHeader } from "./ChatThreadHeader";

// This runner may leave localStorage undefined; stub a minimal in-memory one
// (no-op in CI, where localStorage exists).
if (!("localStorage" in globalThis) || !globalThis.localStorage) {
  const store = new Map<string, string>();
  globalThis.localStorage = {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, String(v)),
    removeItem: (k: string) => void store.delete(k),
    clear: () => store.clear(),
    key: (i: number) => [...store.keys()][i] ?? null,
    get length() {
      return store.size;
    },
  } as Storage;
}

// The header + agent menu read these query hooks; stub them so the test stays
// focused on coachmark gating and never hits the network.
vi.mock("@/api/queries/blueprints", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/queries/blueprints")>();
  return {
    ...actual,
    useAccountBlueprints: (() => ({
      data: { agents: [] },
    })) as unknown as typeof actual.useAccountBlueprints,
  };
});

vi.mock("@/api/queries/deployments", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/queries/deployments")>();
  return {
    ...actual,
    useDeployments: (() => ({
      data: { deployments: [] },
    })) as unknown as typeof actual.useDeployments,
    useDeploymentsSummary: (() => ({
      // Both agents live in the summary; the coachmark gate is driven by which
      // of them is in eligibleDeploymentIds, mirroring AgentDeploymentMenu.
      data: {
        accounts: [
          {
            id: "acct-1",
            name: "acme",
            display_name: "Acme",
            deployments: [
              { id: "dep-1", name: "test-agent", display_name: "Test Agent" },
              { id: "dep-2", name: "other-agent", display_name: "Other Agent" },
            ],
          },
        ],
      },
    })) as unknown as typeof actual.useDeploymentsSummary,
  };
});

const deployment: AgentDeploymentSummary = {
  id: "dep-1",
  name: "test-agent",
  display_name: "Test Agent",
  build_id: "b1",
  created_at: "2026-01-01T00:00:00Z",
};

function renderHeader(
  eligibleDeploymentIds: Set<string>,
  extra?: { sessions?: ChatSession[]; activeConversationId?: string },
) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <ChatThreadHeader
          account="acme"
          deployment={deployment}
          eligibleDeploymentIds={eligibleDeploymentIds}
          sessions={extra?.sessions ?? []}
          activeConversationId={extra?.activeConversationId}
          onSelectSession={vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function session(overrides: Partial<ChatSession> = {}): ChatSession {
  return {
    conversationId: "c1",
    deploymentId: "dep-1",
    title: "Weekend trip to Lisbon",
    updatedAt: "2026-07-10T12:00:00Z",
    ...overrides,
  };
}

const coachmarkText = /switch agents here/i;

function persistedFlag(): boolean {
  return localStorage.getItem("astro:chat-agent-switch-coachmark-seen") === "true";
}

// Clear localStorage so each test starts with the coachmark unseen.
afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe("ChatThreadHeader agent-switch coachmark", () => {
  it("shows the coachmark when unseen and more than one agent is eligible", () => {
    renderHeader(new Set(["dep-1", "dep-2"]));
    expect(screen.getByText(coachmarkText)).toBeInTheDocument();
  });

  it("hides the coachmark for single-agent users", () => {
    renderHeader(new Set(["dep-1"]));
    expect(screen.queryByText(coachmarkText)).not.toBeInTheDocument();
  });

  it("removes the coachmark and persists the flag when dismissed", async () => {
    const user = userEvent.setup();
    renderHeader(new Set(["dep-1", "dep-2"]));

    await user.click(screen.getByRole("button", { name: /dismiss/i }));

    expect(screen.queryByText(coachmarkText)).not.toBeInTheDocument();
    expect(persistedFlag()).toBe(true);
  });

  it("persists the flag when the agent switch menu is opened", async () => {
    const user = userEvent.setup();
    renderHeader(new Set(["dep-1", "dep-2"]));

    await user.click(screen.getByRole("button", { name: /agent menu/i }));

    expect(persistedFlag()).toBe(true);
    expect(screen.queryByText(coachmarkText)).not.toBeInTheDocument();
  });
});

describe("ChatThreadHeader window controls", () => {
  it("surfaces the active conversation title", () => {
    renderHeader(new Set(["dep-1"]), {
      sessions: [session({ title: "Weekend trip to Lisbon" })],
      activeConversationId: "c1",
    });
    expect(screen.getByText("Weekend trip to Lisbon")).toBeInTheDocument();
  });

  it("omits the title when no conversation is active", () => {
    renderHeader(new Set(["dep-1"]), {
      sessions: [session({ title: "Weekend trip to Lisbon" })],
    });
    expect(screen.queryByText("Weekend trip to Lisbon")).not.toBeInTheDocument();
  });

  it("opens the active conversation in a new tab", () => {
    renderHeader(new Set(["dep-1"]), {
      sessions: [session()],
      activeConversationId: "c1",
    });
    const link = screen.getByRole("link", { name: "Open chat in new tab" });
    expect(link).toHaveAttribute("href", "/chat/dep-1?conversation=c1");
    expect(link).toHaveAttribute("target", "_blank");
  });
});
