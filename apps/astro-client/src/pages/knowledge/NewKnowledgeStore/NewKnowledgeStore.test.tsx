import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { renderRoute, mockAuthContext } from "@/test/test-utils";
import NewKnowledgeStore from "./NewKnowledgeStore";

afterEach(cleanup);

describe("NewKnowledgeStore", () => {
  it("keeps the chosen create account through provider selection", async () => {
    const auth = {
      ...mockAuthContext,
      accounts: [
        { id: "acct-1", name: "testuser", type: "personal" },
        {
          id: "acct-2",
          name: "orgaccount",
          display_name: "Org Account",
          type: "organization",
        },
      ],
    };
    const user = userEvent.setup();

    renderRoute(
      [{ path: "/knowledge/new", Component: NewKnowledgeStore }],
      { initialEntries: ["/knowledge/new"], auth },
    );

    await user.click(screen.getByRole("combobox", { name: /create in/i }));
    await user.click(await screen.findByRole("option", { name: /org account/i }));
    await user.click(screen.getByRole("button", { name: /postgresql/i }));

    expect(screen.getByRole("link", { name: "orgaccount" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Connection" })).toBeInTheDocument();
  });
});
