import { renderHook, cleanup, waitFor } from "@testing-library/react";
import { describe, it, expect, afterEach, vi, beforeEach } from "vitest";
import type { AuthResponse, Account } from "./api";
import { AuthProvider } from "./AuthProvider";
import { useAuth } from "./use-auth";

afterEach(cleanup);

// Mock the api module to control what getCurrentUser returns
vi.mock("./api", () => ({
  api: {
    getCurrentUser: vi.fn(),
    getLoginUrl: () => "/auth/login",
    getLogoutUrl: () => "/auth/logout",
    refreshSession: vi.fn(),
  },
}));

import { api } from "./api";
const mockGetCurrentUser = vi.mocked(api.getCurrentUser);

function makeAuthResponse(accounts: Account[]): AuthResponse {
  return {
    user: { id: "u1", email: "a@b.com", first_name: "A", last_name: "B", email_verified: true, created_at: "", updated_at: "" },
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
