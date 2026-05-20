import { ApiClient, type AuthResponse } from "./api";
import { ACTIVE_ACCOUNT_COOKIE, readCookieValue } from "./active-account";

export { ACTIVE_ACCOUNT_COOKIE };

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

export async function getPersonalAccount(request: Request) {
  try {
    const api = createServerApi(request);
    const auth = await getCurrentUserForRequest(request);
    const account = auth.accounts?.find((a) => a.type === "personal");
    return account ? { api, accountName: account.name } : null;
  } catch {
    return null;
  }
}

// Resolves the active account from the `astro:active-account` cookie, falling
// back to the user's personal account when no cookie is set or the cookie
// names an account the user no longer belongs to. Loaders that scope data to
// the active org should use this instead of getPersonalAccount.
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

/**
 * The canonical shape for an account-scoped route loader: resolve the active
 * account from the cookie, fetch the page's main data, return `{ account, data }`
 * (both `null` if the active account couldn't be resolved or the fetch threw).
 *
 * Pages that follow the standard pattern (Agents, Blueprints, Knowledge, etc.)
 * collapse their loader to one call:
 *
 *   export async function loader({ request }: Route.LoaderArgs) {
 *     return loadAccountScoped(request, (api, account) => api.listDeployments(account));
 *   }
 *
 * The matching `usePrimeQueryCache(loaderData, (qc, ld) => { ld.account && qc.setQueryData(key, ld.data) })`
 * reads from `ld.data`. Pages with additional loader inputs (e.g. Insights'
 * `from`/`to`) should keep an inline loader rather than fight the shape.
 */
export async function loadAccountScoped<T>(
  request: Request,
  fetch: (api: ApiClient, account: string) => Promise<T>,
): Promise<{ account: string | null; data: T | null }> {
  const ctx = await getActiveAccount(request);
  if (!ctx) return { account: null, data: null };
  const data = await fetch(ctx.api, ctx.accountName).catch(() => null);
  return { account: ctx.accountName, data };
}
