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

  it("renders account and agent path in breadcrumb", () => {
    renderWithProviders(<BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />);

    expect(screen.getByText("Blueprints")).toBeInTheDocument();
    expect(screen.getByText("signal-watcher")).toBeInTheDocument();

    const accountLinks = screen.getAllByRole("link", { name: /acme/i });
    expect(accountLinks.length).toBeGreaterThan(0);
    for (const link of accountLinks) {
      expect(link).toHaveAttribute("href", "/acme");
    }
  });

  it("uses Explore as the root crumb when navigated from /explore", () => {
    renderWithProviders(
      <BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" />,
      { initialEntries: [{ pathname: "/acme/signal-watcher", state: { from: "/explore" } }] },
    );

    const exploreLink = screen.getByRole("link", { name: /explore/i });
    expect(exploreLink).toHaveAttribute("href", "/explore");
    expect(screen.queryByText("Blueprints")).not.toBeInTheDocument();
  });

  it("links signed-out direct visits back to public discovery", () => {
    renderRoute(
      [{ path: "*", Component: () => <BlueprintDetailBreadcrumb account="acme" blueprintName="signal-watcher" /> }],
      {
        initialEntries: ["/acme/signal-watcher"],
        auth: { ...mockAuthContext, isAuthenticated: false, user: null, accounts: [] },
      },
    );

    expect(screen.getByRole("link", { name: "Blueprints" })).toHaveAttribute("href", "/explore");
  });
});
