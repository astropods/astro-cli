import { useEffect } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ChatWorkspace } from "./ChatWorkspace";

const mountCounts: number[] = [];

vi.mock("./ChatThread", () => ({
  ChatThread: () => {
    useEffect(() => {
      mountCounts.push(1);
    }, []);
    return <div data-testid="chat-thread-stub" />;
  },
}));

vi.mock("@/hooks/use-chat-sessions", () => ({
  useChatSessions: () => ({
    sessions: [],
    recordFirstMessage: vi.fn(),
    isLoading: false,
  }),
}));

// The agent switcher pulls the cross-account deployment summary; stub it so this
// test stays focused on thread remounting.
vi.mock("@/components/agent-detail/AgentDeploymentMenu", () => ({
  AgentDeploymentMenu: () => <div data-testid="agent-menu-stub" />,
}));

let conversationId: string | null = null;

vi.mock("react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router")>();
  return {
    ...actual,
    useSearchParams: () => {
      const params = new URLSearchParams();
      if (conversationId) {
        params.set("conversation", conversationId);
      }
      return [params, vi.fn()] as ReturnType<typeof actual.useSearchParams>;
    },
  };
});

describe("ChatWorkspace", () => {
  it("remounts ChatThread when conversation id changes so thread state stays isolated", () => {
    conversationId = null;
    mountCounts.length = 0;

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    const deployment = {
      id: "dep-1",
      name: "test-agent",
      display_name: "Test Agent",
      build_id: "b1",
      created_at: "2026-01-01T00:00:00Z",
      messaging_web_configured: true,
    };

    const eligibleDeploymentIds = new Set(["dep-1"]);

    const { rerender } = render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ChatWorkspace
            account="testuser"
            deploymentId="dep-1"
            deployment={deployment}
            eligibleDeploymentIds={eligibleDeploymentIds}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(mountCounts).toHaveLength(1);

    conversationId = "ef382a6b-c6c7-4a3e-a57b-b6832759f136";
    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ChatWorkspace
            account="testuser"
            deploymentId="dep-1"
            deployment={deployment}
            eligibleDeploymentIds={eligibleDeploymentIds}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(mountCounts).toHaveLength(2);

    conversationId = "34ac809f-9a55-4b57-b92e-00020720c700";
    rerender(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <ChatWorkspace
            account="testuser"
            deploymentId="dep-1"
            deployment={deployment}
            eligibleDeploymentIds={eligibleDeploymentIds}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(mountCounts).toHaveLength(3);
  });
});
