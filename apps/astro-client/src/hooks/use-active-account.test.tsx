import { describe, it, expect, afterEach, vi } from "vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRoutesStub, Outlet } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { ActiveAccountProvider, useActiveAccount } from "./use-active-account";
import { AuthContext, type AuthContextType } from "@/lib/auth-context";
import { mockAuthContext } from "@/test/test-utils";
import { ACTIVE_ACCOUNT_COOKIE, readCookieValue } from "@/lib/active-account";
import { toast } from "sonner";

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

// Helper component that surfaces the hook's return into the DOM so tests
// can assert against it.
function Probe() {
  const { activeAccount, setActiveAccount } = useActiveAccount();
  return (
    <>
      <span data-testid="active">{activeAccount || "<none>"}</span>
      <button onClick={() => setActiveAccount("acme")}>switch-acme</button>
      <button onClick={() => setActiveAccount(mockAuthContext.accounts[0]!.name)}>switch-personal</button>
    </>
  );
}

interface RenderOpts {
  rootAccount?: string;
  auth?: AuthContextType;
}

function renderWithProviders({ rootAccount = "", auth = mockAuthContext }: RenderOpts = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Stub = createRoutesStub([
    {
      id: "root",
      loader: () => ({ activeAccount: rootAccount }),
      Component: () => (
        <QueryClientProvider client={queryClient}>
          <AuthContext.Provider value={auth}>
            <ActiveAccountProvider>
              <Outlet />
            </ActiveAccountProvider>
          </AuthContext.Provider>
        </QueryClientProvider>
      ),
      children: [{ index: true, Component: Probe }],
    },
  ]);
  return render(<Stub />);
}

afterEach(() => {
  cleanup();
  vi.mocked(toast.error).mockClear();
  document.cookie = `${ACTIVE_ACCOUNT_COOKIE}=;path=/;max-age=0`;
});

// mockAuthContext has personal "testuser"; add acme so we have an org to switch to.
const authWithAcme: AuthContextType = {
  ...mockAuthContext,
  accounts: [
    ...mockAuthContext.accounts,
    { id: "acct-2", name: "acme", type: "organization" },
  ],
};

describe("useActiveAccount — source-of-truth precedence", () => {
  it("uses the root loader's activeAccount as the initial value", async () => {
    renderWithProviders({ rootAccount: "acme", auth: authWithAcme });
    expect(await screen.findByTestId("active")).toHaveTextContent("acme");
  });

  it("falls back to personalAccount when the root loader returns no active account", async () => {
    renderWithProviders({ rootAccount: "", auth: authWithAcme });
    expect(await screen.findByTestId("active")).toHaveTextContent("testuser");
  });

  it("setActiveAccount override beats the loader value after switch", async () => {
    renderWithProviders({ rootAccount: "testuser", auth: authWithAcme });
    expect(await screen.findByTestId("active")).toHaveTextContent("testuser");

    await userEvent.click(screen.getByText("switch-acme"));
    await waitFor(() => {
      expect(screen.getByTestId("active")).toHaveTextContent("acme");
    });
  });

  it("ignores an override that names a non-member account (validOverride gate)", async () => {
    // Render with auth that has NO acme, then trigger the switch-acme button.
    // The override is set, but accounts.some(a => a.name === "acme") is false,
    // so validOverride falls back to null → ssrAccount/personalAccount win.
    renderWithProviders({ rootAccount: "testuser", auth: mockAuthContext });
    expect(await screen.findByTestId("active")).toHaveTextContent("testuser");
    await userEvent.click(screen.getByText("switch-acme"));
    expect(screen.getByTestId("active")).toHaveTextContent("testuser");
  });
});

describe("useActiveAccount — cookie + localStorage side-effects", () => {
  it("setActiveAccount(non-personal) writes the cookie", async () => {
    renderWithProviders({ rootAccount: "testuser", auth: authWithAcme });
    await screen.findByTestId("active");
    await userEvent.click(screen.getByText("switch-acme"));

    expect(readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE)).toBe("acme");
  });

  it("setActiveAccount(personal) clears the cookie", async () => {
    // Start with a cookie set, then switch back to personal.
    document.cookie = `${ACTIVE_ACCOUNT_COOKIE}=acme;path=/`;
    renderWithProviders({ rootAccount: "acme", auth: authWithAcme });
    await screen.findByTestId("active");

    await userEvent.click(screen.getByText("switch-personal"));
    expect(readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE)).toBeNull();
  });
});

describe("useActiveAccount — org-scoped session", () => {
  const authWithScopedAcme = (switchOrg: AuthContextType["switchOrg"]): AuthContextType => ({
    ...mockAuthContext,
    organizationId: "wos-personal",
    switchOrg,
    accounts: [
      { ...mockAuthContext.accounts[0]!, organization_id: "wos-personal" },
      { id: "acct-2", name: "acme", type: "organization", organization_id: "wos-acme" },
    ],
  });

  it("re-mints the session for the target organization before the scope moves", async () => {
    let release = () => {};
    const switchOrg = vi.fn(
      () => new Promise<void>((resolve) => { release = () => resolve(); }),
    );
    renderWithProviders({ rootAccount: "testuser", auth: authWithScopedAcme(switchOrg) });
    await screen.findByTestId("active");

    await userEvent.click(screen.getByText("switch-acme"));

    expect(switchOrg).toHaveBeenCalledWith("wos-acme");
    expect(readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE)).toBeNull();
    expect(screen.getByTestId("active")).toHaveTextContent("testuser");

    await act(async () => { release(); });

    await waitFor(() => {
      expect(readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE)).toBe("acme");
      expect(screen.getByTestId("active")).toHaveTextContent("acme");
    });
  });

  it("stays on the current account when the session cannot be re-scoped", async () => {
    const switchOrg = vi.fn(() => Promise.reject(new Error("switch_failed")));
    renderWithProviders({ rootAccount: "testuser", auth: authWithScopedAcme(switchOrg) });
    await screen.findByTestId("active");

    await userEvent.click(screen.getByText("switch-acme"));

    await waitFor(() => expect(switchOrg).toHaveBeenCalledWith("wos-acme"));
    expect(readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE)).toBeNull();
    expect(screen.getByTestId("active")).toHaveTextContent("testuser");
    expect(toast.error).toHaveBeenCalledWith("Couldn't switch to acme. Try again.");
  });

  it("skips the token switch when the account shares the session organization", async () => {
    const switchOrg = vi.fn(() => Promise.resolve());
    const auth = authWithScopedAcme(switchOrg);
    renderWithProviders({
      rootAccount: "testuser",
      auth: { ...auth, organizationId: "wos-acme" },
    });
    await screen.findByTestId("active");

    await userEvent.click(screen.getByText("switch-acme"));

    await waitFor(() => expect(screen.getByTestId("active")).toHaveTextContent("acme"));
    expect(switchOrg).not.toHaveBeenCalled();
  });
});
