import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "@/test/test-utils";
import { accountKeys } from "@/api/queries/keys";
import { UserFilterBar } from "./UserFilterBar";
import {
  ALL_USERS_KEY,
  UNATTRIBUTED_USER_KEY,
  UNIDENTIFIED_USER_KEY,
  classifyUserId,
} from "./user-classification";

afterEach(cleanup);

const ACCOUNT = "acme";

function seedMembers(
  qc: ReturnType<typeof renderWithProviders>["queryClient"],
  members: Array<{ user_id: string; username: string; display_name: string }>,
) {
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

// Open the radix popover by focusing the embedded search input. The bar
// renders no label/role on the input, so we reach it via the combobox.
function openPopover() {
  const combobox = screen.getByRole("combobox");
  const input = within(combobox).getByRole("textbox");
  fireEvent.focus(input);
}

// ── classifyUserId ────────────────────────────────────────────────────────────

describe("classifyUserId", () => {
  const members = new Set(["u_alice", "u_bob"]);

  it("returns UNATTRIBUTED_USER_KEY for null", () => {
    expect(classifyUserId(null, members)).toBe(UNATTRIBUTED_USER_KEY);
  });

  it("returns UNATTRIBUTED_USER_KEY for undefined", () => {
    expect(classifyUserId(undefined, members)).toBe(UNATTRIBUTED_USER_KEY);
  });

  it("returns UNATTRIBUTED_USER_KEY for empty string", () => {
    expect(classifyUserId("", members)).toBe(UNATTRIBUTED_USER_KEY);
  });

  it("returns the original id when the user is a member", () => {
    expect(classifyUserId("u_alice", members)).toBe("u_alice");
  });

  it("returns UNIDENTIFIED_USER_KEY when the user is not a member", () => {
    expect(classifyUserId("u_outside", members)).toBe(UNIDENTIFIED_USER_KEY);
  });

  it("treats every id as unidentified when there are no members", () => {
    expect(classifyUserId("u_anyone", new Set())).toBe(UNIDENTIFIED_USER_KEY);
  });
});

// ── Entries building ──────────────────────────────────────────────────────────

describe("UserFilterBar entries", () => {
  it("renders members with their display_name and pins the sentinels at the bottom", async () => {
    const { queryClient } = renderWithProviders(
      <UserFilterBar
        account={ACCOUNT}
        presentUserIds={["u_alice", "u_bob", UNIDENTIFIED_USER_KEY, UNATTRIBUTED_USER_KEY]}
        value={[]}
        onValueChange={vi.fn()}
        colorMap={{}}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
      { user_id: "u_bob", username: "bob", display_name: "Bob Martinez" },
    ]);

    openPopover();

    // Display-name resolution lives in TanStack — wait for the re-render.
    await screen.findByText("Alice Chen");

    // The "All users" sentinel is always present at the top.
    const items = screen.getAllByRole("button").filter((b) => {
      const t = b.textContent ?? "";
      return /All users|Alice Chen|Bob Martinez|Unidentified|Infrastructure/.test(t);
    });
    const labels = items.map((i) => i.textContent ?? "");

    expect(labels[0]).toMatch(/All users/);
    // Members sorted alphabetically come next.
    expect(labels[1]).toMatch(/Alice Chen/);
    expect(labels[2]).toMatch(/Bob Martinez/);
    // Sentinels pinned to the bottom — Unidentified before Infrastructure.
    expect(labels[labels.length - 2]).toMatch(/Unidentified/);
    expect(labels[labels.length - 1]).toMatch(/Infrastructure/);
  });

  it("omits the unidentified sentinel when no non-member ids are present", () => {
    const { queryClient } = renderWithProviders(
      <UserFilterBar
        account={ACCOUNT}
        presentUserIds={["u_alice", UNATTRIBUTED_USER_KEY]}
        value={[]}
        onValueChange={vi.fn()}
        colorMap={{}}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    openPopover();

    expect(screen.queryByText(/^Unidentified$/)).not.toBeInTheDocument();
    expect(screen.getByText(/^Infrastructure$/)).toBeInTheDocument();
  });

  it("falls back to the raw id when no member record matches", () => {
    renderWithProviders(
      <UserFilterBar
        account={ACCOUNT}
        presentUserIds={["unknown_id"]}
        value={[]}
        onValueChange={vi.fn()}
        colorMap={{}}
      />,
    );

    openPopover();

    expect(screen.getByText("unknown_id")).toBeInTheDocument();
  });
});

// ── handleChange toggle behavior ──────────────────────────────────────────────

describe("UserFilterBar handleChange", () => {
  it("selecting 'All users' from an empty selection replaces it with [ALL_USERS_KEY]", () => {
    const onValueChange = vi.fn();
    renderWithProviders(
      <UserFilterBar
        account={ACCOUNT}
        presentUserIds={["u_alice"]}
        value={[]}
        onValueChange={onValueChange}
        colorMap={{}}
      />,
    );

    openPopover();
    fireEvent.click(screen.getByText("All users"));

    expect(onValueChange).toHaveBeenCalledTimes(1);
    expect(onValueChange).toHaveBeenCalledWith([ALL_USERS_KEY]);
  });

  it("selecting a specific user while 'All users' is selected removes the ALL_USERS_KEY", async () => {
    const onValueChange = vi.fn();
    const { queryClient } = renderWithProviders(
      <UserFilterBar
        account={ACCOUNT}
        presentUserIds={["u_alice"]}
        value={[ALL_USERS_KEY]}
        onValueChange={onValueChange}
        colorMap={{}}
      />,
    );
    seedMembers(queryClient, [
      { user_id: "u_alice", username: "alice", display_name: "Alice Chen" },
    ]);

    openPopover();
    fireEvent.click(await screen.findByText("Alice Chen"));

    expect(onValueChange).toHaveBeenCalledWith(["u_alice"]);
  });
});
