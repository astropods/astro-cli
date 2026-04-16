import { screen, waitFor, cleanup } from "@testing-library/react";
import { describe, it, expect, afterEach } from "vitest";
import type { AuthContextType } from "@/lib/auth-context";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import ProtectedLayout from "./ProtectedLayout";

afterEach(cleanup);

function renderProtectedLayout(auth: AuthContextType, initialPath = "/protected") {
  return renderRoute(
    [
      {
        path: "/login",
        Component: () => <div>Login page</div>,
      },
      {
        Component: ProtectedLayout,
        children: [
          {
            path: "/protected",
            Component: () => <div>Protected content</div>,
          },
        ],
      },
    ],
    { initialEntries: [initialPath], auth },
  );
}

describe("ProtectedLayout", () => {
  it("renders child route when user is authenticated", async () => {
    renderProtectedLayout(mockAuthContext);

    await waitFor(() => {
      expect(screen.getByText("Protected content")).toBeInTheDocument();
    });
  });

  it("renders nothing while auth is loading", () => {
    renderProtectedLayout({
      ...mockAuthContext,
      isLoading: true,
      isAuthenticated: false,
    });

    expect(screen.queryByText("Protected content")).not.toBeInTheDocument();
    expect(screen.queryByText("Login page")).not.toBeInTheDocument();
  });

  it("redirects to /login when user is not authenticated", async () => {
    renderProtectedLayout({
      ...mockAuthContext,
      isLoading: false,
      isAuthenticated: false,
    });

    await waitFor(() => {
      expect(screen.getByText("Login page")).toBeInTheDocument();
    });
  });
});
