import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { renderRoute } from "@/test/test-utils";
import { DeployedAgentCard } from "./DeployedAgentCard";

afterEach(cleanup);

function renderDeployedAgentCard() {
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
          />
        ),
      },
      {
        path: "/postman/agents/dep-prism",
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
});
