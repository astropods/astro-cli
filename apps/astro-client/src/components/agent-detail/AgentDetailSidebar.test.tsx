import { describe, it, expect } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderWithProviders } from "@/test/test-utils";
import { SidebarCard } from "./AgentDetailSidebar";
import type { Agent, AccountPublic } from "@/lib/api";
import { afterEach } from "vitest";

afterEach(cleanup);

const mockAgent: Agent = {
  name: "signal-watcher",
  account: "acme",
  registry: "default",
  versions: [
    {
      build_id: "a1b2c3d4e5f6g7h8",
      version: "1.2.0",
      spec: {},
      readme: "",
      published_at: "2026-02-28T00:00:00Z",
    },
  ],
};

const mockAccount: AccountPublic = {
  id: "acc-001",
  name: "acme",
  type: "organization",
  owner: {
    first_name: "Steve",
    last_name: "Jobs",
    profile_picture_url: "https://example.com/avatar.png",
  },
  created_at: "2025-06-01T00:00:00Z",
  updated_at: "2026-01-15T00:00:00Z",
};

const baseProps = {
  agent: mockAgent,
  description: "Monitors API behavior and alert conditions.",
  integrations: [
    { id: "github", name: "GitHub" },
    { id: "slack", name: "Slack" },
  ],
  capabilities: ["Read-only access", "Send channel notifications"],
  authors: [{ name: "Steve Jobs", account: "acme" }],
  initialAccountData: mockAccount,
};

describe("AgentDetailSidebar", () => {
  it("renders often used together section when recommended agents are provided", () => {
    renderWithProviders(
      <SidebarCard
        {...baseProps}
        recommendedAgents={[
          {
            slug: "steve_jobs/alert-router",
            account: "steve_jobs",
            name: "alert-router",
            description: "Routes alerts to the right team.",
            rating: 4.6,
            installs: 1203,
          },
        ]}
      />,
    );

    expect(screen.getByText("Often used together")).toBeInTheDocument();
    expect(screen.getByText("alert-router")).toBeInTheDocument();
    expect(screen.getByText("@steve_jobs")).toBeInTheDocument();
  });

  it("does not render often used together section when no recommended agents", () => {
    renderWithProviders(
      <SidebarCard
        {...baseProps}
        recommendedAgents={[]}
      />,
    );

    expect(screen.queryByText("Often used together")).not.toBeInTheDocument();
  });
});
