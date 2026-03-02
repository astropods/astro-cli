import { screen, waitFor, cleanup } from "@testing-library/react";
import { describe, it, expect, afterEach } from "vitest";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import Admin from "./Admin";

afterEach(cleanup);

function renderAdmin(permissions: string[] = []) {
  return renderRoute(
    [{ path: "/admin", Component: Admin }],
    {
      initialEntries: ["/admin"],
      auth: { ...mockAuthContext, permissions },
    },
  );
}

describe("Admin page", () => {
  it("renders admin content when user has admin:view permission", async () => {
    renderAdmin(["admin:view"]);

    await waitFor(() => {
      expect(
        screen.getByText("Admin dashboard coming soon."),
      ).toBeInTheDocument();
    });
  });

  it("shows no-permission message when user lacks admin:view", async () => {
    renderAdmin([]);

    await waitFor(() => {
      expect(
        screen.getByText("You don't have permission to view this page."),
      ).toBeInTheDocument();
    });
  });

  it("shows no-permission message with unrelated permissions", async () => {
    renderAdmin(["agents:read", "agents:deploy"]);

    await waitFor(() => {
      expect(
        screen.getByText("You don't have permission to view this page."),
      ).toBeInTheDocument();
    });
  });
});
