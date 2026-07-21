import { describe, expect, it, afterEach } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSearchParams } from "react-router";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { renderRoute } from "@/test/test-utils";
import ChatPage from "./Chat";

// Surfaces the ?conversation= param so a test can assert what "New chat" routes to.
function ChatPageWithConversationProbe() {
  const [params] = useSearchParams();
  return (
    <>
      <div data-testid="conversation-param">
        {params.get("conversation") ?? "none"}
      </div>
      <ChatPage />
    </>
  );
}

const eligibleDeployment = {
  id: "dep-chat-1",
  name: "chat-agent",
  display_name: "Chat Agent",
  build_id: "b1",
  messaging_web_configured: true,
  created_at: "2026-01-01T00:00:00Z",
};

const notChattableDeployment = {
  id: "dep-plain-1",
  name: "plain-agent",
  display_name: "Plain Agent",
  build_id: "b2",
  messaging_web_configured: false,
  created_at: "2026-01-01T00:00:00Z",
};

const summaryWithAccount = {
  accounts: [
    {
      id: "acct-1",
      name: "testuser",
      type: "user",
      display_name: "Test User",
      deployments: [],
    },
  ],
};

// jsdom lacks Element.scrollTo, which assistant-ui calls during the thread's
// auto-scroll on mount (the new-chat test renders a populated thread).
if (!Element.prototype.scrollTo) {
  Element.prototype.scrollTo = () => {};
}

afterEach(() => {
  cleanup();
});

describe("ChatPage", () => {
  it("shows the agent switcher and conversation history for chat-eligible deployments", async () => {
    server.use(
      http.get("/api/v1/deployments", () =>
        HttpResponse.json({
          deployments: [eligibleDeployment],
          count: 1,
        }),
      ),
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({ value: "active", reason: "ready", details: "" }),
      ),
      http.get("/api/v1/deployments/:id/runtime", () =>
        HttpResponse.json({
          runtime: { ready: 1, replicas: 1, messaging_reachable: true },
        }),
      ),
      http.get("/api/v1/deployments/summary", () =>
        HttpResponse.json({
          accounts: [
            {
              id: "acct-1",
              name: "testuser",
              type: "user",
              display_name: "Test User",
              deployments: [],
            },
          ],
        }),
      ),
      http.get("/api/v1/deployments/:id/chat/conversations", () =>
        HttpResponse.json({ conversations: [] }),
      ),
    );

    renderRoute(
      [
        {
          path: "/chat/:deploymentId?",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: ChatPage as any,
        },
      ],
      { initialEntries: ["/chat/dep-chat-1"] },
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Agent menu" }),
      ).toBeInTheDocument();
    });
    expect(screen.getAllByText("Chat Agent").length).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: "Chat history" }),
    ).toBeInTheDocument();
  });

  it("new chat clears the conversation from the URL instead of seeding a client-generated id", async () => {
    // Regression: pre-seeding a client-generated conversation id in the URL made
    // the send lazily create the row while the SSE stream subscribed to it in
    // parallel, so the stream could 404 the not-yet-created conversation and hang.
    // New chat must instead go to a blank chat (no conversation param) so the row
    // is created server-side before the stream subscribes.
    server.use(
      http.get("/api/v1/deployments", () =>
        HttpResponse.json({ deployments: [eligibleDeployment], count: 1 }),
      ),
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({ value: "active", reason: "ready", details: "" }),
      ),
      http.get("/api/v1/deployments/:id/runtime", () =>
        HttpResponse.json({
          runtime: { ready: 1, replicas: 1, messaging_reachable: true },
        }),
      ),
      http.get("/api/v1/deployments/summary", () =>
        HttpResponse.json(summaryWithAccount),
      ),
      // One existing conversation, so the workspace auto-selects it (URL gets the
      // param) — the state from which "New chat" must clear back to blank.
      http.get("/api/v1/deployments/:id/chat/conversations", () =>
        HttpResponse.json({
          conversations: [
            {
              conversation_id: "existing-conv-1",
              title: "Earlier chat",
              updated_at: "2026-07-10T00:00:00Z",
              assistant_streaming: false,
            },
          ],
        }),
      ),
      http.get("/api/v1/deployments/:id/chat/conversations/:conversationId", () =>
        HttpResponse.json({
          conversation_id: "existing-conv-1",
          messages: [],
          assistant_streaming: false,
        }),
      ),
    );

    renderRoute(
      [
        {
          path: "/chat/:deploymentId?",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: ChatPageWithConversationProbe as any,
        },
      ],
      { initialEntries: ["/chat/dep-chat-1"] },
    );

    // The existing conversation is auto-selected first.
    await waitFor(() => {
      expect(screen.getByTestId("conversation-param")).toHaveTextContent(
        "existing-conv-1",
      );
    });

    await userEvent.click(screen.getByRole("button", { name: "New chat" }));

    // New chat routes to a blank chat — no conversation id in the URL (the row is
    // created server-side on first send), never a client-generated id.
    await waitFor(() => {
      expect(screen.getByTestId("conversation-param")).toHaveTextContent("none");
    });
  });

  it("new chat stays blank when arriving with a conversation already selected (does not bounce to latest)", async () => {
    // Regression: auto-select marked itself done only when it actually selected, so
    // arriving with a conversation in the URL left the guard unset — a later "New
    // chat" (which clears the URL) then re-triggered auto-select and bounced back
    // to the latest (possibly still-streaming) conversation instead of a blank chat.
    server.use(
      http.get("/api/v1/deployments", () =>
        HttpResponse.json({ deployments: [eligibleDeployment], count: 1 }),
      ),
      http.get("/api/v1/deployments/:id/status", () =>
        HttpResponse.json({ value: "active", reason: "ready", details: "" }),
      ),
      http.get("/api/v1/deployments/:id/runtime", () =>
        HttpResponse.json({
          runtime: { ready: 1, replicas: 1, messaging_reachable: true },
        }),
      ),
      http.get("/api/v1/deployments/summary", () =>
        HttpResponse.json(summaryWithAccount),
      ),
      http.get("/api/v1/deployments/:id/chat/conversations", () =>
        HttpResponse.json({
          conversations: [
            {
              conversation_id: "existing-conv-1",
              title: "Earlier chat",
              updated_at: "2026-07-10T00:00:00Z",
              assistant_streaming: false,
            },
          ],
        }),
      ),
      http.get("/api/v1/deployments/:id/chat/conversations/:conversationId", () =>
        HttpResponse.json({
          conversation_id: "existing-conv-1",
          messages: [],
          assistant_streaming: false,
        }),
      ),
    );

    // Arrive with the conversation already in the URL — the case the old guard missed.
    renderRoute(
      [
        {
          path: "/chat/:deploymentId?",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: ChatPageWithConversationProbe as any,
        },
      ],
      { initialEntries: ["/chat/dep-chat-1?conversation=existing-conv-1"] },
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "New chat" })).toBeInTheDocument();
    });
    expect(screen.getByTestId("conversation-param")).toHaveTextContent(
      "existing-conv-1",
    );

    await userEvent.click(screen.getByRole("button", { name: "New chat" }));

    await waitFor(() => {
      expect(screen.getByTestId("conversation-param")).toHaveTextContent("none");
    });
    // And it must stay blank — auto-select must not bounce it back to the latest.
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.getByTestId("conversation-param")).toHaveTextContent("none");
  });

  it("shows the not-chattable empty state when agents exist but none have web chat enabled", async () => {
    server.use(
      http.get("/api/v1/deployments", () =>
        HttpResponse.json({
          deployments: [notChattableDeployment],
          count: 1,
        }),
      ),
      http.get("/api/v1/deployments/summary", () =>
        HttpResponse.json(summaryWithAccount),
      ),
    );

    renderRoute(
      [
        {
          path: "/chat/:deploymentId?",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: ChatPage as any,
        },
      ],
      { initialEntries: ["/chat"] },
    );

    await waitFor(() => {
      expect(
        screen.getByText("No agents are connected to chat yet"),
      ).toBeInTheDocument();
    });
  });

  it("shows the no-agents empty state when there are no deployments at all", async () => {
    server.use(
      http.get("/api/v1/deployments", () =>
        HttpResponse.json({ deployments: [], count: 0 }),
      ),
      http.get("/api/v1/deployments/summary", () =>
        HttpResponse.json(summaryWithAccount),
      ),
      // no-chat-agents renders NoAgentsState, which reads the account's
      // blueprints to pick its CTA — stub it so the card settles out of its
      // loading spinner.
      http.get("/api/v1/agents/:account", () =>
        HttpResponse.json({ agents: [], count: 0 }),
      ),
    );

    renderRoute(
      [
        {
          path: "/chat/:deploymentId?",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: ChatPage as any,
        },
      ],
      { initialEntries: ["/chat"] },
    );

    await waitFor(() => {
      expect(
        screen.getByText("No agents to chat with yet"),
      ).toBeInTheDocument();
    });
  });

  it("shows the error state (not a resting empty state) when a deployment read fails", async () => {
    server.use(
      http.get("/api/v1/deployments/summary", () =>
        HttpResponse.json(summaryWithAccount),
      ),
      // A failed per-account read reports 0 deployments; without the isError
      // branch this would masquerade as "no agents to chat with yet".
      http.get("/api/v1/deployments", () =>
        HttpResponse.json({ error: "boom" }, { status: 500 }),
      ),
    );

    renderRoute(
      [
        {
          path: "/chat/:deploymentId?",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          Component: ChatPage as any,
        },
      ],
      { initialEntries: ["/chat"] },
    );

    await waitFor(() => {
      expect(
        screen.getByText("Couldn't load your agents"),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: /try again/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("No agents to chat with yet"),
    ).not.toBeInTheDocument();
  });
});
