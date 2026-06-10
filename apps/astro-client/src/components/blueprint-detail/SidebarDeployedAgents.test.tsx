import { describe, it, expect, afterEach } from "vitest";
import { screen, waitFor, cleanup, fireEvent } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { renderWithProviders } from "@/test/test-utils";
import { server } from "@/test/msw/server";
import { SidebarDeployedAgents } from "./SidebarDeployedAgents";
import type { AgentDeploymentSummary, DeploymentsListResponse } from "@/lib/api";

afterEach(cleanup);

function deployment(overrides: Partial<AgentDeploymentSummary>): AgentDeploymentSummary {
  return {
    id: "dep-default",
    name: "inbox-triage",
    display_name: "Default deployment",
    build_id: "build-001",
    namespace: "astro-ns",
    status: "Running",
    created_at: "2026-05-01T00:00:00Z",
    ...overrides,
  };
}

function mockDeploymentsResponse(deployments: AgentDeploymentSummary[]) {
  const body: DeploymentsListResponse = { deployments, count: deployments.length };
  server.use(
    http.get("/api/v1/deployments", () => HttpResponse.json(body)),
  );
}

describe("SidebarDeployedAgents", () => {
  it("returns null when the viewer is not a member of the account", () => {
    mockDeploymentsResponse([deployment({ build_id: "build-001" })]);
    renderWithProviders(
      <SidebarDeployedAgents
        account="someone-else"
        blueprintName="inbox-triage"
        buildIds={["build-001"]}
      />,
    );
    expect(screen.queryByText("Deployed agents")).not.toBeInTheDocument();
  });

  it("returns null when no deployments match the blueprint's buildIds", async () => {
    mockDeploymentsResponse([deployment({ build_id: "other-build" })]);
    renderWithProviders(
      <SidebarDeployedAgents
        account="testuser"
        blueprintName="inbox-triage"
        buildIds={["build-001"]}
      />,
    );
    await waitFor(() => {
      expect(screen.queryByText("Deployed agents")).not.toBeInTheDocument();
    });
  });

  it("renders matching deployments sorted newest first", async () => {
    mockDeploymentsResponse([
      deployment({ id: "dep-old", display_name: "Older deploy", created_at: "2026-01-01T00:00:00Z" }),
      deployment({ id: "dep-new", display_name: "Newer deploy", created_at: "2026-06-01T00:00:00Z" }),
    ]);

    renderWithProviders(
      <SidebarDeployedAgents
        account="testuser"
        blueprintName="inbox-triage"
        buildIds={["build-001"]}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Deployed agents")).toBeInTheDocument();
    });

    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveTextContent("Newer deploy");
    expect(links[1]).toHaveTextContent("Older deploy");
  });

  it("collapses deployments beyond the visible limit behind a toggle", async () => {
    mockDeploymentsResponse(
      Array.from({ length: 6 }, (_, i) =>
        deployment({
          id: `dep-${i}`,
          display_name: `Deploy ${i}`,
          created_at: `2026-06-${String(i + 1).padStart(2, "0")}T00:00:00Z`,
        }),
      ),
    );

    renderWithProviders(
      <SidebarDeployedAgents
        account="testuser"
        blueprintName="inbox-triage"
        buildIds={["build-001"]}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Deployed agents")).toBeInTheDocument();
    });

    // 4 rows visible by default; the remaining 2 are hidden behind the toggle.
    expect(screen.getByRole("button", { name: /show 2 more/i })).toBeInTheDocument();
  });

  it("expands hidden deployments when the toggle is clicked", async () => {
    mockDeploymentsResponse(
      Array.from({ length: 6 }, (_, i) =>
        deployment({
          id: `dep-${i}`,
          display_name: `Deploy ${i}`,
          created_at: `2026-06-${String(i + 1).padStart(2, "0")}T00:00:00Z`,
        }),
      ),
    );

    renderWithProviders(
      <SidebarDeployedAgents
        account="testuser"
        blueprintName="inbox-triage"
        buildIds={["build-001"]}
      />,
    );

    const toggle = await screen.findByRole("button", { name: /show 2 more/i });
    fireEvent.click(toggle);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /show less/i })).toBeInTheDocument();
    });
    expect(screen.getAllByRole("link")).toHaveLength(6);
  });
});
