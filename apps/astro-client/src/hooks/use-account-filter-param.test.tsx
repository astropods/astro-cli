import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { describe, expect, it } from "vitest";
import { AuthContext, type AuthContextType } from "@/lib/auth-context";
import { mockAuthContext } from "@/test/test-utils";
import { useAccountFilterParam } from "./use-account-filter-param";

const auth: AuthContextType = {
  ...mockAuthContext,
  accounts: [
    { id: "acct-1", name: "team,west", type: "organization" },
    { id: "acct-2", name: "acme", type: "organization" },
  ],
};

function Probe() {
  const [value] = useAccountFilterParam();
  const { search } = useLocation();
  return (
    <>
      <span data-testid="value">{JSON.stringify(value)}</span>
      <span data-testid="search">{search}</span>
    </>
  );
}

describe("useAccountFilterParam", () => {
  it("preserves account names containing commas", async () => {
    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter initialEntries={["/?account=team%2Cwest"]}>
          <Probe />
        </MemoryRouter>
      </AuthContext.Provider>,
    );

    expect(await screen.findByTestId("value")).toHaveTextContent('["team,west"]');
  });

  it("removes stale accounts without dropping valid accounts or other params", async () => {
    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter initialEntries={["/?account=team%2Cwest&account=ghost&q=agent"]}>
          <Probe />
        </MemoryRouter>
      </AuthContext.Provider>,
    );

    await waitFor(() => {
      const params = new URLSearchParams(screen.getByTestId("search").textContent ?? "");
      expect(params.getAll("account")).toEqual(["team,west"]);
      expect(params.get("q")).toBe("agent");
    });
  });
});
