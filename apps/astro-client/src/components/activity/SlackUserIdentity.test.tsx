import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { renderWithProviders } from "@/test/test-utils";
import { SlackUserIdentity } from "./SlackUserIdentity";

afterEach(cleanup);

describe("SlackUserIdentity", () => {
  it("uses the shared Slack label and short tooltip copy", async () => {
    const user = userEvent.setup();

    renderWithProviders(
      <SlackUserIdentity
        user={{
          user_id: "U09SLACK01",
          user_details: {
            kind: "slack",
            team_id: "T09TEAM",
            display_name: "Acme User",
            username: "acme.user",
          },
        }}
        variant="trace"
      />,
    );

    const link = screen.getByRole("link", { name: /Acme User/ });
    expect(link).toHaveAttribute("href", "slack://user?team=T09TEAM&id=U09SLACK01");

    await user.hover(link);
    expect((await screen.findAllByText("Slack User")).length).toBeGreaterThan(0);
    expect(screen.queryByText(/isn't a member/)).not.toBeInTheDocument();
  });

  it("matches Astro identity emphasis in trace rows", () => {
    const { container } = renderWithProviders(
      <SlackUserIdentity
        user={{
          user_id: "U09SLACK01",
          user_details: {
            kind: "slack",
            team_id: "T09TEAM",
            display_name: "Acme Trace User",
            username: "acme.trace.user",
            avatar_url: "https://example.com/acme.png",
          },
        }}
        variant="trace"
      />,
    );

    const avatar = container.querySelector("img");
    expect(avatar).toHaveClass("size-5");
    expect(avatar).not.toHaveClass("opacity-60");
    expect(screen.getByText("Acme Trace User")).toHaveClass("text-foreground");
  });
});
