import { describe, expect, it, afterEach } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "@/test/msw/server";
import { renderRoute } from "@/test/test-utils";
import ChatPage from "./Chat";

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
