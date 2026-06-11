import { afterEach, describe, expect, it } from "vitest";
import { screen, cleanup } from "@testing-library/react";
import { UserBadge } from "./UserBadge";
import { renderWithProviders } from "@/test/test-utils";
import { accountKeys } from "@/api/queries/keys";

afterEach(cleanup);

const ACCOUNT = "acme";

// UserBadge picks one of three identity sources for a given WorkOS id:
//   1. The in-account member list (preferred — carries Slack workspace
//      data and the most up-to-date display name).
//   2. user_details fallback props (displayName + username) — used when
//      the member list doesn't have the user, e.g. cross-account public-
//      blueprint spend.
//   3. "Unknown user" — nothing else matched.
//
// The tests below pin the precedence and verify the avatar URL + profile
// link target track whichever source won.

function seedMembers(qc: ReturnType<typeof renderWithProviders>["queryClient"], members: Array<{ user_id: string; username: string; display_name: string }>) {
  qc.setQueryData(accountKeys.members(ACCOUNT), {
    members: members.map((m) => ({
      account_id: "acct-acme",
      user_id: m.user_id,
      role: "member",
      status: "active",
      username: m.username,
      display_name: m.display_name,
      created_at: "2025-01-01T00:00:00Z",
      slack_workspaces: [],
    })),
  });
}

describe("UserBadge", () => {
  it("uses the in-account member when one is found", async () => {
    const { queryClient } = renderWithProviders(
      <UserBadge
        userId="user_01HXX_bob"
        account={ACCOUNT}
        // Fallback props are present too, but the member entry should
        // win — it's the freshest source.
        displayName="Stale Bob"
        username="stale-bob"
        linkToProfile
      />,
    );
    seedMembers(queryClient, [
      { user_id: "user_01HXX_bob", username: "bob", display_name: "Bob Smith" },
    ]);

    expect(await screen.findByText("Bob Smith")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /Bob Smith/ });
    expect(link.getAttribute("href")).toBe("/bob");
    // Stale fallback shouldn't leak into the rendered output.
    expect(screen.queryByText("Stale Bob")).not.toBeInTheDocument();
    expect(link.getAttribute("href")).not.toBe("/stale-bob");
  });

  it("falls back to user_details props when no member is found", async () => {
    const { queryClient } = renderWithProviders(
      <UserBadge
        userId="user_01HXX_carol"
        account={ACCOUNT}
        displayName="Carol Chen"
        username="carol"
        linkToProfile
      />,
    );
    // No matching member — exercises the fallback path (cross-account user).
    seedMembers(queryClient, []);

    expect(await screen.findByText("Carol Chen")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /Carol Chen/ });
    expect(link.getAttribute("href")).toBe("/carol");
  });

  it("uses the username when no displayName is provided", async () => {
    const { queryClient } = renderWithProviders(
      <UserBadge
        userId="user_01HXX_dave"
        account={ACCOUNT}
        username="dave"
        linkToProfile
      />,
    );
    seedMembers(queryClient, []);

    // No display name → handle shows up as both the label and the slug.
    expect(await screen.findByText("dave")).toBeInTheDocument();
    expect(screen.getByRole("link").getAttribute("href")).toBe("/dave");
  });

  it("renders Unknown user when neither member nor fallback resolves", async () => {
    const { queryClient } = renderWithProviders(
      <UserBadge userId="user_01HXX_ghost" account={ACCOUNT} />,
    );
    seedMembers(queryClient, []);

    expect(await screen.findByText("Unknown user")).toBeInTheDocument();
    // No link should be rendered — there's no slug to route to.
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("renders nothing useful when userId is empty", () => {
    renderWithProviders(<UserBadge userId={undefined} account={ACCOUNT} displayName="Ignored" username="ignored" />);
    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText("Ignored")).not.toBeInTheDocument();
  });
});
