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

afterEach(() => {
  cleanup();
});

describe("ChatPage", () => {
  it("lists deployments with messaging_web_configured", async () => {
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
        HttpResponse.json({ accounts: [] }),
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
      expect(screen.getByText("Chat Agent")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("button", { name: "New conversation" }),
    ).toBeInTheDocument();
  });
});
