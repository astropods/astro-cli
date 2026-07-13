import { describe, it, expect, beforeEach } from "vitest";
import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { AuthContext } from "@/lib/auth-context";
import { mockAuthContext } from "@/test/test-utils";
import { server } from "@/test/msw/server";
import { CustomAccessControl } from "./CustomAccessControl";
import type { AccountMembersResponse, AuthGrant } from "@/lib/api";

const TARGET_ACCOUNT = "acct-1";

const membersResponse: AccountMembersResponse = {
  members: [
    {
      account_id: TARGET_ACCOUNT,
      user_id: "user-1",
      role: "admin",
      status: "active",
      username: "testuser",
      display_name: "Test User",
      created_at: "2025-01-01T00:00:00Z",
      slack_workspaces: [],
    },
    {
      account_id: TARGET_ACCOUNT,
      user_id: "user-2",
      role: "member",
      status: "active",
      username: "alice",
      display_name: "Alice Chen",
      created_at: "2025-01-01T00:00:00Z",
      slack_workspaces: [],
    },
  ],
};

beforeEach(() => {
  server.use(
    http.get("/api/v1/accounts/:account/members", () => HttpResponse.json(membersResponse)),
  );
});

function Harness({
  initialPublic = false,
  initialGrants = [],
}: {
  initialPublic?: boolean;
  initialGrants?: AuthGrant[];
}) {
  const [isPublic, setIsPublic] = useState(initialPublic);
  const [grants, setGrants] = useState<AuthGrant[]>(initialGrants);
  return (
    <CustomAccessControl
      isPublic={isPublic}
      onPublicChange={setIsPublic}
      grants={grants}
      onGrantsChange={setGrants}
      targetAccount={TARGET_ACCOUNT}
    />
  );
}

function renderHarness(props?: { initialPublic?: boolean; initialGrants?: AuthGrant[] }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const user = userEvent.setup();
  render(
    <AuthContext.Provider value={mockAuthContext}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <Harness {...props} />
        </MemoryRouter>
      </QueryClientProvider>
    </AuthContext.Provider>,
  );
  return { user };
}

describe("CustomAccessControl", () => {
  it("shows the access dropdown reflecting an anyone grant, with no list or warning", () => {
    renderHarness({ initialGrants: [{ anyone: true }] });
    expect(screen.getByRole("combobox")).toHaveTextContent("Anyone with an Astro account");
    expect(screen.queryByRole("button", { name: /add member/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/this interface is open/i)).not.toBeInTheDocument();
  });

  it("shows the open-access warning in public mode", () => {
    renderHarness({ initialPublic: true });
    expect(screen.getByRole("combobox")).toHaveTextContent("Public");
    expect(screen.getByText(/this interface is open/i)).toBeInTheDocument();
  });

  it("locks the deploying user's own row and allows removing others", async () => {
    renderHarness({ initialGrants: [{ user_id: "user-1" }, { user_id: "user-2" }] });
    expect(screen.getByRole("combobox")).toHaveTextContent("Invited only");
    expect(await screen.findByText(/\(you\)/)).toBeInTheDocument();
    // Two grants, but only the non-self grant exposes a remove control.
    expect(screen.getAllByRole("button", { name: /remove grant/i })).toHaveLength(1);
  });

  it("opens the member picker directly from Add member (no org/user submenu)", async () => {
    const { user } = renderHarness({ initialGrants: [] });
    expect(screen.getByText(/no one is invited yet/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /add member/i }));
    expect(screen.getByPlaceholderText(/search by account name/i)).toBeInTheDocument();
    expect(screen.queryByRole("menuitem")).not.toBeInTheDocument();
  });

  it("auto-invites and locks the deploying user when switching to Invited only", async () => {
    const { user } = renderHarness({ initialGrants: [{ anyone: true }] });
    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByRole("option", { name: "Invited only" }));
    expect(await screen.findByText(/\(you\)/)).toBeInTheDocument();
    // Self is the only grant and is not removable.
    expect(screen.queryByRole("button", { name: /remove grant/i })).not.toBeInTheDocument();
  });

  it("clears grants and warns when switching to Public", async () => {
    const { user } = renderHarness({ initialGrants: [{ anyone: true }] });
    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByRole("option", { name: "Public" }));
    expect(screen.getByRole("combobox")).toHaveTextContent("Public");
    expect(screen.getByText(/this interface is open/i)).toBeInTheDocument();
  });
});
