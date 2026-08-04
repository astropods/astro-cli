import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { describe, expect, it } from "vitest";
import { AuthContext, type AuthContextType } from "@/lib/auth-context";
import { mockAuthContext } from "@/test/test-utils";
import {
  useAccountFilterParam,
  usePersistentAccountFilterParam,
} from "./use-account-filter-param";

const auth: AuthContextType = {
  ...mockAuthContext,
  accounts: [
    { id: "acct-1", name: "team,west", type: "organization" },
    { id: "acct-2", name: "acme", type: "organization" },
  ],
};

const personalAuth: AuthContextType = {
  ...auth,
  accounts: [
    { id: "acct-personal", name: "personal-user", type: "personal" },
    ...auth.accounts,
  ],
};

function Probe() {
  const [value, , hasExplicitSelection] = useAccountFilterParam();
  const { search } = useLocation();
  return (
    <>
      <span data-testid="value">{JSON.stringify(value)}</span>
      <span data-testid="search">{search}</span>
      <span data-testid="explicit">{String(hasExplicitSelection)}</span>
    </>
  );
}

function PersistentProbe() {
  const [value] = usePersistentAccountFilterParam("blueprints");
  const { search } = useLocation();
  return (
    <>
      <span data-testid="value">{JSON.stringify(value)}</span>
      <span data-testid="search">{search}</span>
    </>
  );
}

describe("useAccountFilterParam", () => {
  it("defaults a bare list URL to the personal account", async () => {
    render(
      <AuthContext.Provider value={personalAuth}>
        <MemoryRouter initialEntries={["/?q=agent"]}>
          <Probe />
        </MemoryRouter>
      </AuthContext.Provider>,
    );

    expect(await screen.findByTestId("value")).toHaveTextContent('["personal-user"]');
    expect(screen.getByTestId("search")).toHaveTextContent("?q=agent");
    expect(screen.getByTestId("explicit")).toHaveTextContent("false");
  });

  it("resolves an all-stale URL to personal on the first render", () => {
    const renderedValues: string[][] = [];

    function FirstRenderProbe() {
      const [value, , hasExplicitSelection] = useAccountFilterParam();
      renderedValues.push([...value, String(hasExplicitSelection)]);
      return null;
    }

    render(
      <AuthContext.Provider value={personalAuth}>
        <MemoryRouter initialEntries={["/?account=old-org"]}>
          <FirstRenderProbe />
        </MemoryRouter>
      </AuthContext.Provider>,
    );

    expect(renderedValues[0]).toEqual(["personal-user", "true"]);
  });

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

  it("removes a complete explicit selection as the canonical all-accounts scope", async () => {
    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter initialEntries={["/?account=acme&account=team%2Cwest&q=agent"]}>
          <Probe />
        </MemoryRouter>
      </AuthContext.Provider>,
    );
    await waitFor(() => {
      const params = new URLSearchParams(screen.getByTestId("search").textContent ?? "");
      expect(screen.getByTestId("value")).toHaveTextContent("[]");
      expect(params.getAll("account")).toEqual([]);
      expect(params.get("scope")).toBe("all");
      expect(params.get("q")).toBe("agent");
    });
  });
});

describe("usePersistentAccountFilterParam", () => {
  it("restores the Blueprint account selection when the route mounts again", async () => {
    localStorage.setItem("astro:page-filters:blueprints", "scope=all");
    const first = render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter initialEntries={["/?account=acme"]}>
          <PersistentProbe />
        </MemoryRouter>
      </AuthContext.Provider>,
    );
    await waitFor(() => {
      expect(localStorage.getItem("astro:page-filters:blueprints")).toBe("account=acme");
    });
    first.unmount();

    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter initialEntries={["/"]}>
          <PersistentProbe />
        </MemoryRouter>
      </AuthContext.Provider>,
    );
    await waitFor(() => {
      expect(screen.getByTestId("value")).toHaveTextContent('["acme"]');
      expect(screen.getByTestId("search")).toHaveTextContent("?account=acme");
    });
  });
});
