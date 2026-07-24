import { ApiClient, type AuthResponse } from "./api";
import { ACTIVE_ACCOUNT_COOKIE, readCookieValue } from "./active-account";

export function createServerApi(request: Request): ApiClient {
  const apiUrl = process.env.API_URL || "http://localhost:8080";
  const cookie = request.headers.get("cookie") || "";
  return new ApiClient(apiUrl, apiUrl, { cookie });
}

// Per-request memo of `/me`: React Router runs root + page loaders in
// parallel and both helpers below need the auth response, but the backend
// shouldn't get two identical hits per navigation.
const authByRequest = new WeakMap<Request, Promise<AuthResponse>>();

export function getCurrentUserForRequest(request: Request): Promise<AuthResponse> {
  let p = authByRequest.get(request);
  if (!p) {
    p = createServerApi(request).getCurrentUser();
    authByRequest.set(request, p);
  }
  return p;
}

// Resolves the active account from the `astro:active-account` cookie, falling
// back to the user's personal account when no cookie is set or the cookie
// names an account the user no longer belongs to.
export async function getActiveAccount(request: Request) {
  try {
    const api = createServerApi(request);
    const auth = await getCurrentUserForRequest(request);
    if (!auth.accounts?.length) return null;
    const cookieName = readCookieValue(request.headers.get("cookie"), ACTIVE_ACCOUNT_COOKIE);
    const match = cookieName ? auth.accounts.find((a) => a.name === cookieName) : null;
    const account = match ?? auth.accounts.find((a) => a.type === "personal") ?? auth.accounts[0];
    return account ? { api, accountName: account.name } : null;
  } catch {
    return null;
  }
}
