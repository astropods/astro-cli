import { screen, waitFor, cleanup } from "@testing-library/react";
import { describe, it, expect, afterEach } from "vitest";
import { useSearchParams } from "react-router";
import type { AuthContextType } from "@/lib/auth-context";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import ProtectedLayout from "./ProtectedLayout";

afterEach(cleanup);

function LoginStub() {
  const [search] = useSearchParams();
  return <div>Login page redirect={search.get("redirect")}</div>;
}

function renderProtectedLayout(
  auth: AuthContextType,
  initialPath = "/protected",
) {
  return renderRoute(
    [
      {
        path: "/login",
        Component: LoginStub,
      },
      {
        Component: ProtectedLayout,
        children: [
          {
            path: "/protected",
            Component: () => <div>Protected content</div>,
          },
          {
            path: "/protected/inbox",
            Component: () => <div>Inbox</div>,
          },
        ],
      },
    ],
    { initialEntries: [initialPath], auth },
  );
}

describe("ProtectedLayout", () => {
  it("user sees protected content when authenticated", async () => {
    renderProtectedLayout(mockAuthContext);

    await waitFor(() => {
      expect(screen.getByText("Protected content")).toBeInTheDocument();
    });
  });

  it("user does not see protected content or login flash while auth is resolving", async () => {
    renderProtectedLayout({
      ...mockAuthContext,
      isLoading: true,
      isAuthenticated: false,
    });

    await waitFor(() => {
      expect(screen.queryByText("Protected content")).not.toBeInTheDocument();
    });
    expect(screen.queryByText(/Login page/)).not.toBeInTheDocument();
  });

  it("unauthenticated user is redirected to /login with their original path as redirect param", async () => {
    renderProtectedLayout(
      { ...mockAuthContext, isLoading: false, isAuthenticated: false },
      "/protected/inbox",
    );

    await waitFor(() => {
      expect(
        screen.getByText("Login page redirect=/protected/inbox"),
      ).toBeInTheDocument();
    });
  });
});
