import { useState } from "react";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { mockAuthContext, renderRoute } from "@/test/test-utils";
import { AccountFilter } from "./AccountFilter";

const auth = {
  ...mockAuthContext,
  accounts: [
    { id: "personal", name: "testuser", display_name: "Test User", type: "personal" },
    { id: "org", name: "acme", display_name: "Acme", type: "organization" },
  ],
};

function Harness() {
  const [accounts, setAccounts] = useState(["testuser"]);
  return (
    <>
      <span data-testid="selection">{JSON.stringify(accounts)}</span>
      <AccountFilter value={accounts} onChange={setAccounts} />
    </>
  );
}

describe("AccountFilter", () => {
  it("emits the canonical empty value when the last unselected account is selected", async () => {
    const user = userEvent.setup();
    renderRoute([{ path: "/", Component: Harness }], { auth });

    const trigger = screen.getByRole("button", { name: "Filter by account" });
    await user.click(trigger);
    await user.click(screen.getByRole("button", { name: /Acme/ }));

    expect(screen.getByTestId("selection")).toHaveTextContent("[]");
    expect(trigger).toHaveTextContent("All accounts");
  });
});
