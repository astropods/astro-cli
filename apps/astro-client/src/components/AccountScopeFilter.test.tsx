import { useState } from "react";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { ACTIVE_ACCOUNT_COOKIE, readCookieValue } from "@/lib/active-account";
import { setOrgSwitchTarget } from "@/lib/org-switch-progress";
import { useActiveAccount } from "@/hooks/use-active-account";
import { mockAuthContext, renderRoute } from "@/test/test-utils";
import { AccountScopeFilter } from "./AccountScopeFilter";

afterEach(() => {
  cleanup();
  setOrgSwitchTarget(null);
  document.cookie = `${ACTIVE_ACCOUNT_COOKIE}=;path=/;max-age=0`;
});

const auth = {
  ...mockAuthContext,
  accounts: [
    { id: "personal", name: "testuser", display_name: "Test User", type: "personal" },
    { id: "org", name: "acme", display_name: "Acme", type: "organization" },
  ],
};

function Harness() {
  const [account, setAccount] = useState("testuser");
  const { activeAccount } = useActiveAccount();
  return (
    <>
      <span data-testid="active-account">{activeAccount}</span>
      <AccountScopeFilter value={account} onChange={setAccount} />
    </>
  );
}

describe("AccountScopeFilter", () => {
  it("reports the selection and leaves the active account to its caller", async () => {
    const user = userEvent.setup();
    renderRoute([{ path: "/", Component: Harness }], { auth });

    const trigger = screen.getByRole("combobox", { name: "Scope by account" });
    expect(trigger).toHaveTextContent("Test User");
    expect(screen.getByTestId("active-account")).toHaveTextContent("testuser");
    expect(readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE)).toBeNull();

    await user.click(trigger);
    await user.click(screen.getByRole("option", { name: /Acme/ }));

    expect(trigger).toHaveTextContent("Acme");
    expect(screen.getByTestId("active-account")).toHaveTextContent("testuser");
    expect(readCookieValue(document.cookie, ACTIVE_ACCOUNT_COOKIE)).toBeNull();
  });

  it("shows the account a switch is moving to while the session re-scopes", async () => {
    const user = userEvent.setup();
    renderRoute([{ path: "/", Component: Harness }], { auth });

    const trigger = screen.getByRole("combobox", { name: "Scope by account" });
    expect(trigger).toHaveTextContent("Test User");

    setOrgSwitchTarget("acme");

    await waitFor(() => expect(trigger).toHaveTextContent("Acme"));
    expect(trigger).toHaveAttribute("aria-busy", "true");

    await user.click(trigger);
    expect(screen.queryByRole("option", { name: /Test User/ })).not.toBeInTheDocument();
  });
});
