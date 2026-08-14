import { describe, it, expect, afterEach } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { renderRoute, renderWithProviders, mockAuthContext } from "@/test/test-utils";
import { BlueprintDetailBreadcrumb } from "./BlueprintDetailBreadcrumb";

afterEach(cleanup);

describe("BlueprintDetailBreadcrumb", () => {
  it("renders Like and Share action badges", () => {
    renderWithProviders(<BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />);

    expect(screen.getAllByRole("button", { name: /heart/i })).not.toHaveLength(0);
    expect(screen.getAllByRole("button", { name: /share/i })).not.toHaveLength(0);
  });

  it("renders a single back link instead of an account path", () => {
    renderWithProviders(<BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />);

    expect(screen.getAllByRole("link", { name: /back to blueprints/i })[0]).toHaveAttribute(
      "href",
      "/blueprints",
    );
    expect(screen.queryByText("signal-watcher")).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^acme$/i })).not.toBeInTheDocument();
  });

  it("points back to Explore when navigated from /explore", () => {
    renderWithProviders(
      <BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />,
      { initialEntries: [{ pathname: "/acme/signal-watcher", state: { from: "/explore" } }] },
    );

    expect(screen.getAllByRole("link", { name: /back to explore/i })[0]).toHaveAttribute(
      "href",
      "/explore",
    );
  });

  it("links signed-out direct visits back to public discovery", () => {
    renderRoute(
      [{ path: "*", Component: () => <BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" /> }],
      {
        initialEntries: ["/acme/signal-watcher"],
        auth: { ...mockAuthContext, isAuthenticated: false, user: null, accounts: [] },
      },
    );

    expect(screen.getAllByRole("link", { name: /back to blueprints/i })[0]).toHaveAttribute(
      "href",
      "/explore",
    );
  });
});
