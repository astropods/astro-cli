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
  const tree = (deploymentId: string) => (
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <MemoryRouter>
        <ChatWorkspace
          account="acme"
          deploymentId={deploymentId}
          deployment={{
            id: deploymentId,
            name: "test-agent",
            display_name: "Test Agent",
            build_id: "b1",
            created_at: "2026-01-01T00:00:00Z",
            messaging_web_configured: true,
          }}
          eligibleDeploymentIds={new Set([deploymentId])}
        />
      </MemoryRouter>
    </QueryClientProvider>
  );

  it("keeps ChatThread mounted across conversation switches, remounts on agent switch", () => {
    conversationId = null;
    mountCounts.length = 0;

    const { rerender } = render(tree("dep-1"));
    expect(mountCounts).toHaveLength(1);

    // Switching conversations must NOT remount ChatThread. The chat runtime
    // re-scopes in place (use-deployment-chat reacts to the conversationId
    // change), so the Stop/Send button reflects the active chat and the live
    // stream survives — remounting per conversation is what caused the button
    // state to carry over / lag between chats.
    conversationId = "ef382a6b-c6c7-4a3e-a57b-b6832759f136";
    rerender(tree("dep-1"));
    expect(mountCounts).toHaveLength(1);

    conversationId = "34ac809f-9a55-4b57-b92e-00020720c700";
    rerender(tree("dep-1"));
    expect(mountCounts).toHaveLength(1);

    // Switching agents DOES remount — the runtime is keyed per deployment.
    rerender(tree("dep-2"));
    expect(mountCounts).toHaveLength(2);
  });
});
