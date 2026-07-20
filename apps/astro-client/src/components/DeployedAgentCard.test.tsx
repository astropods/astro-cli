import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { renderRoute } from "@/test/test-utils";
import { DeployedAgentCard } from "./DeployedAgentCard";

afterEach(cleanup);

function renderDeployedAgentCard(overrides: Partial<Parameters<typeof DeployedAgentCard>[0]> = {}) {
  return renderRoute(
    [
      {
        path: "/",
        Component: () => (
          <DeployedAgentCard
            account="postman"
            name="prism"
            displayName="Prism"
            deploymentId="dep-prism"
            requestSeries={[0, 1, 2]}
            {...overrides}
          />
        ),
      },
      {
        // Agent cards use the same builder-focused default as deploymentPath.
        path: "/postman/agents/dep-prism/deployments",
        Component: () => <div data-testid="agent-detail">Agent detail</div>,
      },
      {
        path: "/postman/prism",
        Component: () => <div data-testid="blueprint-detail">Blueprint detail</div>,
      },
    ],
    { auth: null },
  );
}

describe("DeployedAgentCard", () => {
  it("navigates to the agent detail page when the card shell is clicked", async () => {
    const user = userEvent.setup();
    renderDeployedAgentCard();

    await user.click(screen.getByRole("link", { name: "View details for Prism" }));

    await waitFor(() => {
      expect(screen.getByTestId("agent-detail")).toBeInTheDocument();
    });
  });

  it("navigates to the agent detail page from keyboard activation", async () => {
    const user = userEvent.setup();
    renderDeployedAgentCard();
    const card = screen.getByRole("link", { name: "View details for Prism" });

    card.focus();
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(screen.getByTestId("agent-detail")).toBeInTheDocument();
    });
  });

  it("lets inner clickable elements take precedence over the card click", async () => {
    const user = userEvent.setup();
    renderDeployedAgentCard();

    await user.click(screen.getByRole("link", { name: "postman/prism" }));

    await waitFor(() => {
      expect(screen.getByTestId("blueprint-detail")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("agent-detail")).not.toBeInTheDocument();
  });

  it("does not expose a Delete agent option in the card menu", async () => {
    const user = userEvent.setup();
    renderDeployedAgentCard();

    await user.click(screen.getByRole("button", { name: "Agent options" }));

    expect(screen.queryByRole("menuitem", { name: "Delete agent" })).not.toBeInTheDocument();
  });

  it("keeps target-length deployment titles fully visible", () => {
    renderDeployedAgentCard({ displayName: "Sohum's Slack Test Bot" });
    expect(screen.getByText("Sohum's Slack Test Bot")).toBeInTheDocument();

    cleanup();
    renderDeployedAgentCard({ displayName: "Feature Flag Assistant" });
    expect(screen.getByText("Feature Flag Assistant")).toBeInTheDocument();
  });

  it("renders longer deployment titles with the main centered title classes", () => {
    const longName = "VerylongagentnameVeeryVerylongagentname";
    renderDeployedAgentCard({ displayName: longName });

    const title = screen.getByText("VerylongagentnameVeery\u2026");
    expect(title).toHaveClass("text-heading-2", "text-balance", "text-center", "text-foreground");
    expect(title).not.toHaveClass("truncate");
  });
});
