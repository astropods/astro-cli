import { renderHook, cleanup, waitFor, act } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { describe, it, expect, afterEach, vi, beforeEach } from "vitest";
import { api, ApiRequestError, type AuthResponse, type Account } from "./api";
import { AuthProvider } from "./AuthProvider";
import { useAuth } from "./use-auth";
import { useAccountFilterParam } from "@/hooks/use-account-filter-param";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const mockGetCurrentUser = vi.spyOn(api, "getCurrentUser");
const mockSwitchOrg = vi.spyOn(api, "switchOrg");

function makeAuthResponse(
  accounts: Account[],
  userId = "u1",
): AuthResponse {
  return {
    user: { id: userId, email: "a@b.com", first_name: "A", last_name: "B", email_verified: true, created_at: "", updated_at: "" },
    session_id: "s1",
    permissions: [],
    expires_at: new Date(Date.now() + 86400000).toISOString(),
    accounts,
  };
}

function renderUseAuth() {
  return renderHook(() => useAuth(), {
    wrapper: ({ children }) => <AuthProvider>{children}</AuthProvider>,
  });
}

function renderUseAuthWithAccountFilter() {
  return renderHook(
    () => {
      const auth = useAuth();
      const [selectedAccounts] = useAccountFilterParam("insights");
      return { auth, selectedAccounts };
    },
    {
      wrapper: ({ children }) => (
        <MemoryRouter initialEntries={["/insights?account=shared"]}>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      ),
    },
  );
}

describe("AuthProvider needsOnboarding", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("sets needsOnboarding=false when user has a personal account", async () => {
    mockGetCurrentUser.mockResolvedValue(
      makeAuthResponse([{ id: "acct-1", name: "testuser", type: "personal" }]),
    );

    const { result } = renderUseAuth();

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsOnboarding).toBe(false);
  });

  it("sets needsOnboarding=true when user has only org accounts", async () => {
    mockGetCurrentUser.mockResolvedValue(
      makeAuthResponse([{ id: "acct-org", name: "my-org", type: "organization" }]),
    );

    const { result } = renderUseAuth();

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsOnboarding).toBe(true);
  });

  it("sets needsOnboarding=true when user has no accounts", async () => {
    mockGetCurrentUser.mockResolvedValue(makeAuthResponse([]));

    const { result } = renderUseAuth();

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsOnboarding).toBe(true);
  });

  it("sets needsOnboarding=false when user has both personal and org accounts", async () => {
    mockGetCurrentUser.mockResolvedValue(
      makeAuthResponse([
        { id: "acct-1", name: "testuser", type: "personal" },
        { id: "acct-org", name: "my-org", type: "organization" },
      ]),
    );

    const { result } = renderUseAuth();

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsOnboarding).toBe(false);
  });
});

describe("AuthProvider switchOrg", () => {
  const locationReplace = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("location", {
      href: "",
      replace: locationReplace,
      pathname: "/settings/org/foo",
      search: "",
    });
    mockGetCurrentUser.mockResolvedValue(makeAuthResponse([]));
  });

  it("clears persisted page filters after switching organizations", async () => {
    localStorage.setItem(
      "astro:page-filter-owner",
      JSON.stringify(["u1", null]),
    );
    localStorage.setItem("unrelated", "keep");
    mockGetCurrentUser.mockResolvedValue(
      makeAuthResponse([
        { id: "old-shared", name: "shared", type: "organization" },
      ]),
    );
    mockSwitchOrg.mockResolvedValue({
      ...makeAuthResponse([
        { id: "new-shared", name: "shared", type: "organization" },
      ]),
      organization_id: "org-2",
    });

    const { result } = renderUseAuthWithAccountFilter();
    await waitFor(() => {
      expect(result.current.auth.isLoading).toBe(false);
      expect(result.current.selectedAccounts).toEqual(["shared"]);
      expect(
        new URLSearchParams(
          localStorage.getItem("astro:page-filters:insights") ?? "",
        ).get("account"),
      ).toBe("shared");
    });

    await act(async () => {
      await result.current.auth.switchOrg("org-2");
    });

    await waitFor(() => {
      expect(result.current.selectedAccounts).toEqual([]);
    });
    expect(localStorage.getItem("astro:page-filters:insights")).toBeNull();
    expect(localStorage.getItem("unrelated")).toBe("keep");
  });

  it("redirects to login with current path when session is expired", async () => {
    mockSwitchOrg.mockRejectedValue(
      new ApiRequestError({ error: "session_expired", error_description: "Session has expired" }, 401),
    );

    const { result } = renderUseAuth();
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(async () => {
      await result.current.switchOrg("org-2").catch(() => {});
    });

    expect(locationReplace).toHaveBeenCalledWith(
      expect.stringContaining("/auth/login"),
    );
  });

  it("throws for non-session errors so the caller can show an error state", async () => {
    mockSwitchOrg.mockRejectedValue(
      new ApiRequestError({ error: "switch_failed", error_description: "Failed to switch organization" }, 400),
    );

    const { result } = renderUseAuth();
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await expect(
      act(async () => {
        await result.current.switchOrg("org-2");
      }),
    ).rejects.toThrow("Failed to switch organization");
  });
});

describe("AuthProvider authentication", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetCurrentUser.mockResolvedValue(makeAuthResponse([]));
  });

  it("clears persisted page filters when the authenticated user changes", async () => {
    localStorage.setItem(
      "astro:page-filter-owner",
      JSON.stringify(["u1", null]),
    );
    localStorage.setItem("unrelated", "keep");
    mockGetCurrentUser.mockResolvedValue(
      makeAuthResponse([
        { id: "old-shared", name: "shared", type: "organization" },
      ]),
    );

    const { result } = renderUseAuthWithAccountFilter();
    await waitFor(() => {
      expect(result.current.auth.isLoading).toBe(false);
      expect(result.current.selectedAccounts).toEqual(["shared"]);
    });

    act(() => {
      result.current.auth.hydrateAuth(
        makeAuthResponse(
          [{ id: "new-shared", name: "shared", type: "organization" }],
          "u2",
        ),
      );
    });

    await waitFor(() => expect(result.current.selectedAccounts).toEqual([]));
    expect(localStorage.getItem("astro:page-filters:insights")).toBeNull();
    expect(localStorage.getItem("unrelated")).toBe("keep");
  });
});

describe("AuthProvider logout", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("location", { href: "" });
    mockGetCurrentUser.mockResolvedValue(makeAuthResponse([]));
  });

  it("clears persisted page filters before redirecting", async () => {
    localStorage.setItem("astro:page-filters:insights", "account=secret");
    localStorage.setItem("unrelated", "keep");

    const { result } = renderUseAuth();
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => result.current.logout());

    expect(localStorage.getItem("astro:page-filters:insights")).toBeNull();
    expect(localStorage.getItem("unrelated")).toBe("keep");
  });
});
