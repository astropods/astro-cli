import { describe, expect, it, vi } from "vitest";
import { loader } from "./Index";
import { getCurrentUserForRequest } from "@/lib/api.server";
import type { AuthResponse } from "@/lib/api";

vi.mock("@/lib/api.server", () => ({
  getCurrentUserForRequest: vi.fn(),
}));

const mockedGetCurrentUserForRequest = vi.mocked(getCurrentUserForRequest);

function request() {
  return new Request("https://app.astropods.com/");
}

const authResponse: AuthResponse = {
  user: {
    id: "user-1",
    email: "user@example.com",
    email_verified: true,
    created_at: "2026-06-26T00:00:00Z",
    updated_at: "2026-06-26T00:00:00Z",
  },
  session_id: "session-1",
  permissions: [],
  expires_at: "2026-06-27T00:00:00Z",
  accounts: [{ id: "acct-1", name: "testuser", type: "personal" }],
};

async function redirectedPath(response: Response) {
  return new URL(response.headers.get("location") ?? "", "https://app.astropods.com").pathname;
}

describe("Index loader", () => {
  it("redirects signed-in users to account blueprints", async () => {
    mockedGetCurrentUserForRequest.mockResolvedValueOnce(authResponse);

    const response = await loader({ request: request() });

    expect(await redirectedPath(response)).toBe("/blueprints");
  });

  it("redirects signed-out users to public discovery", async () => {
    mockedGetCurrentUserForRequest.mockRejectedValueOnce(new Error("unauthorized"));

    const response = await loader({ request: request() });

    expect(await redirectedPath(response)).toBe("/explore");
  });
});
