import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderRoute } from "@/test/test-utils";
import { DeployedAgentCard } from "./DeployedAgentCard";

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  SVGSVGElement.prototype,
  "getAnimations",
);

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (originalGetAnimations) {
    Object.defineProperty(
      SVGSVGElement.prototype,
      "getAnimations",
      originalGetAnimations,
    );
  } else {
    Reflect.deleteProperty(SVGSVGElement.prototype, "getAnimations");
  }
});

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
  it("does not activate star animations until the card is hovered", async () => {
    const user = userEvent.setup();
    const animation = {
      animationName: "card-star-drift",
      pause: vi.fn(),
      play: vi.fn(),
      playbackRate: 0,
      playState: "paused",
    } as unknown as Animation;
    const getAnimations = vi.fn(() => [animation]);
    Object.defineProperty(SVGSVGElement.prototype, "getAnimations", {
      configurable: true,
      value: getAnimations,
    });

    renderDeployedAgentCard();
    const card = screen.getByRole("link", { name: "View details for Prism" });

    expect(getAnimations).not.toHaveBeenCalled();
    expect(animation.play).not.toHaveBeenCalled();

    await user.hover(card);
    expect(getAnimations).toHaveBeenCalledTimes(1);
    expect(animation.play).toHaveBeenCalledTimes(1);

    await user.unhover(card);
    await user.hover(card);
    expect(getAnimations).toHaveBeenCalledTimes(1);
  });

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

  it("renders a non-navigable access provisioning state", () => {
    renderDeployedAgentCard({ accessProvisioning: true });

    expect(screen.getByText("Setting up access")).toBeInTheDocument();
    expect(screen.getByRole("status", { name: "Deployment access is being configured" })).toHaveTextContent("Updates automatically");
    expect(screen.queryByRole("link", { name: "View details for Prism" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Agent options" })).not.toBeInTheDocument();
  });

  it("uses the delayed copy when access setup exceeds the normal window", () => {
    renderDeployedAgentCard({ accessProvisioning: true, accessProvisioningDelayed: true });

    expect(screen.getByText("Still setting up access")).toBeInTheDocument();
    expect(screen.getByText("Secure access is taking longer than usual.")).toBeInTheDocument();
  });

  it("offers a retry when access setup reaches its terminal state", async () => {
    const user = userEvent.setup();
    const onRetryAccess = vi.fn();
    renderDeployedAgentCard({
      accessProvisioning: true,
      accessProvisioningStalled: true,
      onRetryAccess,
    });

    expect(screen.getByText("Access setup needs attention")).toBeInTheDocument();
    expect(screen.getByText("We couldn’t finish setting up secure access.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Retry access setup" }));
    expect(onRetryAccess).toHaveBeenCalledOnce();
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
