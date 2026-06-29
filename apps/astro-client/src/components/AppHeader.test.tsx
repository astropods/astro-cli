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
});
