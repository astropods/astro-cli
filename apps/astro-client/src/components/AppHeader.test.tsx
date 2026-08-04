import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import { AppHeader } from "./AppHeader";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import type { AuthContextType } from "@/lib/auth";

function renderHeader(auth: AuthContextType) {
  return renderRoute(
    [{ path: "*", Component: AppHeader }],
    { initialEntries: ["/explore"], auth },
  );
}

const signedOutAuth: AuthContextType = {
  ...mockAuthContext,
  user: null,
  sessionId: null,
  organizationId: null,
  role: null,
  permissions: [],
  expiresAt: null,
  isAuthenticated: false,
  accounts: [],
};

const loadingSignedOutAuth: AuthContextType = {
  ...signedOutAuth,
  isLoading: true,
};

const multiAccountAuth: AuthContextType = {
  ...mockAuthContext,
  accounts: [
    ...mockAuthContext.accounts,
    { id: "acct-test-org", name: "test-org", type: "organization" },
    { id: "acct-other-org", name: "other-org", type: "organization" },
  ],
};

describe("AppHeader navigation", () => {
  it("does not show a Blueprints nav tab to signed-out users", () => {
    renderHeader(signedOutAuth);

    expect(screen.queryByRole("link", { name: "Blueprints" })).not.toBeInTheDocument();
    // The public explorer is still reachable via the Explore action.
    expect(screen.getByRole("link", { name: "Explore" })).toHaveAttribute("href", "/explore");
  });

  it("keeps pending signed-out visitors on public-safe navigation", () => {
    renderHeader(loadingSignedOutAuth);

    expect(screen.queryByRole("link", { name: "Blueprints" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Agents" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Feedback" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Log in" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Get started" })).toBeInTheDocument();
  });

  it("keeps signed-in users on the account-scoped blueprints page", () => {
    renderHeader(mockAuthContext);

    expect(screen.getByRole("link", { name: "Blueprints" })).toHaveAttribute("href", "/blueprints");
    expect(screen.getByRole("link", { name: "Agents" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Feedback" })).toBeInTheDocument();
  });

  it("links directly to each primitive's persisted account filter", () => {
    localStorage.setItem("astro:page-filters:blueprints", "scope=all");
    localStorage.setItem("astro:page-filters:agents", "account=test-org");
    localStorage.setItem(
      "astro:page-filters:knowledge",
      "account=old-org&account=other-org",
    );

    renderHeader(multiAccountAuth);

    expect(screen.getByRole("link", { name: "Blueprints" })).toHaveAttribute(
      "href",
      "/blueprints?scope=all",
    );
    expect(screen.getByRole("link", { name: "Agents" })).toHaveAttribute(
      "href",
      "/agents?account=test-org",
    );
    expect(screen.getByRole("link", { name: "Knowledge" })).toHaveAttribute(
      "href",
      "/knowledge?account=other-org",
    );
  });

  it("drops an all-stale persisted account filter from primitive links", () => {
    localStorage.setItem("astro:page-filters:agents", "account=old-org");

    renderHeader(multiAccountAuth);

    expect(screen.getByRole("link", { name: "Agents" })).toHaveAttribute(
      "href",
      "/agents",
    );
  });
});
