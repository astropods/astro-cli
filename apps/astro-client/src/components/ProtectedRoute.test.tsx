import { screen, waitFor, cleanup } from "@testing-library/react";
import { describe, it, expect, afterEach, vi } from "vitest";
import type { AuthContextType } from "@/lib/auth-context";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import { ProtectedRoute } from "./ProtectedRoute";

afterEach(cleanup);

function Content() {
  return <div>Protected content</div>;
}

function renderProtected(auth: AuthContextType) {
  return renderRoute(
    [
      {
        path: "/",
        Component: () => (
          <ProtectedRoute>
            <Content />
          </ProtectedRoute>
        ),
      },
      {
        path: "/onboarding",
        Component: () => <div>Onboarding page</div>,
      },
    ],
    { initialEntries: ["/"], auth },
  );
}

describe("ProtectedRoute", () => {
  it("renders children when user has a personal account", async () => {
    renderProtected(mockAuthContext);

    await waitFor(() => {
      expect(screen.getByText("Protected content")).toBeInTheDocument();
    });
  });

  it("redirects to onboarding when user has org accounts but no personal account", async () => {
    renderProtected({
      ...mockAuthContext,
      accounts: [{ id: "acct-org", name: "my-org", type: "organization" }],
      needsOnboarding: true,
    });

    await waitFor(() => {
      expect(screen.getByText("Onboarding page")).toBeInTheDocument();
    });
  });

  it("redirects to onboarding when user has no accounts at all", async () => {
    renderProtected({
      ...mockAuthContext,
      accounts: [],
      needsOnboarding: true,
    });

    await waitFor(() => {
      expect(screen.getByText("Onboarding page")).toBeInTheDocument();
    });
  });

  it("renders nothing while auth is loading", () => {
    renderProtected({
      ...mockAuthContext,
      isLoading: true,
      isAuthenticated: false,
    });

    expect(screen.queryByText("Protected content")).not.toBeInTheDocument();
    expect(screen.queryByText("Onboarding page")).not.toBeInTheDocument();
  });

  it("calls login when user is not authenticated", async () => {
    const login = vi.fn();
    renderProtected({
      ...mockAuthContext,
      isLoading: false,
      isAuthenticated: false,
      login,
    });

    await waitFor(() => {
      expect(login).toHaveBeenCalled();
    });
  });
});
