import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { renderRoute } from "@/test/test-utils";
import { DeleteDeploymentDialog } from "@/components/DeleteDeploymentDialog";
import { DeployedAgentCard } from "./DeployedAgentCard";

afterEach(cleanup);

function AgentCardWithDeleteDialog() {
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <>
      <DeployedAgentCard
        account="postman"
        name="prism"
        displayName="Prism"
        deploymentId="dep-prism"
        requestSeries={[0, 1, 2]}
        onDeleteRequest={() => setDeleteOpen(true)}
      />
      {deleteOpen && (
        <DeleteDeploymentDialog
          open={deleteOpen}
          onOpenChange={setDeleteOpen}
          deploymentId="dep-prism"
          deploymentName="prism"
          displayName="Prism"
          account="postman"
        />
      )}
    </>
  );
}

function renderDeployedAgentCard() {
  return renderRoute(
    [
      {
        path: "/",
        Component: AgentCardWithDeleteDialog,
      },
      {
        // Card-level click on `/agents` routes to the Monitor tab; the
        // "Manage agent" button is the one that still hits the deployments
        // tab. Tests here cover the card-shell click, so this fixture
        // mirrors the Monitor destination.
        path: "/postman/agents/dep-prism/monitor",
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

  it("does not navigate when interacting with the delete confirmation dialog", async () => {
    const user = userEvent.setup();
    renderDeployedAgentCard();

    await user.click(screen.getByRole("button", { name: "Agent options" }));
    await user.click(screen.getByRole("menuitem", { name: "Delete agent" }));

    expect(screen.getByRole("dialog", { name: "Delete Prism" })).toBeInTheDocument();

    await user.click(screen.getByRole("checkbox"));

    expect(screen.getByRole("dialog", { name: "Delete Prism" })).toBeInTheDocument();
    expect(screen.queryByTestId("agent-detail")).not.toBeInTheDocument();
  });
});
